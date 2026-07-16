package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/go-ldap/ldap/v3"
	scim_filter_parser "github.com/scim2/filter-parser/v2"

	"github.com/josegomezr/scim-schreiber-ldap/internal/utils"
	"github.com/josegomezr/scim-schreiber-ldap/internal/uuidgenerator"
)

type UserHandler struct {
	cfg           *Config
	uuidGenerator uuidgenerator.UUIDGenerator
}

func (h UserHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {

	slog.Info("POST /v2/Users", "request", attributes)
	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	username, ok := attributes["userName"].(string)
	if !ok {
		// Shouldn't really get here because then the schema validation failed before.
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusBadRequest,
		}
	}

	if ldapCtx.searchUserByUsername(username) != nil {
		return scim.Resource{}, errors.ScimErrorUniqueness
	}

	if !h.cfg.AllowUserCreation {
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusForbidden,
		}
	}

	ldapAttributes := h.resourceToLdap(utils.FlattenAttrs(attributes))
	externalId := attributes["externalId"].(string)

	ldapAttributes["employeeNumber"] = []string{"-1"}
	ldapAttributes["uuid"] = []string{h.uuidGenerator.NewUUID(externalId)}

	dn, err := ldapCtx.CreateUser(externalId, ldapAttributes)

	if err != nil {
		slog.Error("Failed to create user", "error", err)
		return scim.Resource{}, err
	}

	entry := ldapCtx.searchUserByDN(dn)
	return ldapEntryToUserResource(entry), nil
}

// TODO(josegomezr): sometimes IDP's don't really _delete_ things. We gotta find a creative way to
//
//	                  issue a deletion from SCIM.
//						 Model is set to be deleted, task to roll updates is scheduled in the background
//	                  by the time the task is executed, the record does not exist in the DB anymore.
func (h UserHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE /v2/Users", "id", id)

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	if u := ldapCtx.searchUserByUsername(id); u != nil {
		// TODO delete the user
	}

	return errors.ScimError{
		Status: http.StatusNotImplemented,
	}
}

func (h UserHandler) Get(r *http.Request, id string) (scim.Resource, error) {

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	entry := ldapCtx.searchUserByUsername(id)

	if entry == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	return ldapEntryToUserResource(entry), nil
}

func principalFromFilter(dafilter scim_filter_parser.Expression) (string, error) {
	if dafilter == nil {
		return "", nil
	}
	f, ok := dafilter.(*scim_filter_parser.AttributeExpression)
	if !ok {
		return "", fmt.Errorf("only single expressions are supported")
	}
	if f.Operator != "eq" {
		return "", fmt.Errorf("only operator 'eq' is supported in filters")
	}
	if f.AttributePath.AttributeName != "userName" {
		return "", fmt.Errorf("only 'userName' is supported in filters")
	}
	return f.CompareValue.(string), nil
}

func (h UserHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Page{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	if params.Count == 0 {
		userCount, err := ldapCtx.CountUsers()

		if err != nil {
			return scim.Page{}, errors.ScimErrorInternal
		}

		return scim.Page{
			TotalResults: userCount,
		}, nil
	}

	resources := make([]scim.Resource, 0)

	uid := "*"
	if params.FilterValidator != nil {
		filterVal, err := principalFromFilter(params.FilterValidator.GetFilter())
		if err != nil {
			return scim.Page{}, err
		}
		if filterVal != "" {
			uid = filterVal
		}
	}

	users, err := ldapCtx.searchUsers(uid, "uid")

	if err != nil {
		return scim.Page{}, errors.ScimErrorInternal
	}

	i := 1

	for entry, err := range users {
		if err != nil {
			slog.Error("An error occurred while querying LDAP", "err", err)
			return scim.Page{}, errors.ScimError{Status: http.StatusInternalServerError, Detail: "LDAP Query failed"}
		}
		// Ldap pagination does not support start index. So skip until we find the correct entry
		if i > (params.StartIndex + params.Count - 1) {
			// If we wanted to provide the correct result in TotalResults we'd actually have to keep counting here.
			break
		}

		if i >= params.StartIndex {
			resources = append(resources, ldapEntryToUserResource(entry))
		}
		i++
	}

	return scim.Page{
		TotalResults: i,
		Resources:    resources,
	}, nil
}

func (h UserHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {

	if len(operations) != 1 {
		return scim.Resource{}, errors.ScimErrorBadRequest("Must be exactly one operation")
	}

	operation := operations[0]
	if operation.Op != scim.PatchOperationReplace || operation.Path != nil {
		return scim.Resource{}, errors.ScimErrorBadRequest("Must be replace operation at root")
	}

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	attributes := operation.Value.(map[string]interface{})

	if attributes["userName"] != id {
		return scim.Resource{}, errors.ScimErrorBadRequest("Username must match id")
	}

	entry := ldapCtx.searchUserByUsername(id)
	if entry == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	slog.Debug("Found user", "entry", entry)

	if entry.DN != attributes["externalId"] {
		slog.Warn("Mismatched DN ", "entryDn", entry.DN, "externalId", attributes["externalId"])
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	slog.Info("Updating user details.", "uid", entry.GetAttributeValue("uid"))

	replaces := filterChanged(h.resourceToLdap(attributes), entry)

	if len(replaces) == 0 {
		slog.Info("No changes.")
		return ldapEntryToUserResource(entry), nil
	}

	slog.Info("Updating user details.", "replaces", replaces)

	err := ldapCtx.UpdateEntry(entry.DN, nil, nil, replaces)

	if err != nil {
		slog.Error("Error updating entry", "error", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	// Get updated entry
	entry = ldapCtx.searchUserByUsername(id)
	// return resource with replaced attributes
	return ldapEntryToUserResource(entry), nil
}

func filterChanged(replaces map[string][]string, entry *ldap.Entry) map[string][]string {
	cleaned_replaces := make(map[string][]string, len(replaces))

	for attributeName, attributeValue := range replaces {
		if !slices.Equal(entry.GetAttributeValues(attributeName), attributeValue) {
			cleaned_replaces[attributeName] = attributeValue
		}
	}
	return cleaned_replaces
}

func (h UserHandler) resourceToLdap(attributes map[string]interface{}) map[string][]string {
	s := scimMailToLdap(attributes)

	name := attributes["name"].(map[string]interface{})

	address := map[string]interface{}{}
	if addresses, ok := attributes["addresses"]; ok && len(addresses.([]interface{})) > 0 {
		address = addresses.([]interface{})[0].(map[string]interface{})
	}

	var organization []string
	if ext, ok := attributes["urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"].(map[string]interface{}); ok {
		organization = getOptionalAttribute(ext, "organization")
	} else if orgAttr, ok := attributes["urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:organization"]; ok {
		organization = []string{orgAttr.(string)}
	}

	var sshPublicKey []string
	if ext, ok := attributes["urn:ietf:params:scim:schemas:extension:suse:2.0:User"].(map[string]interface{}); ok {
		sshPublicKey = getOptionalAttributeSlice(ext, "sshPublicKey", []string{})
	} else if sshAttr, ok := attributes["urn:ietf:params:scim:schemas:extension:suse:2.0:User:sshPublicKey"]; ok {
		if slice, ok := sshAttr.([]interface{}); ok {
			for _, v := range slice {
				sshPublicKey = append(sshPublicKey, v.(string))
			}
		}
	}

	replaces := map[string][]string{
		"isActive":     {LdapBoolToString(attributes["active"].(bool))},
		"cn":           {name["formatted"].(string)},
		"title":        getOptionalAttribute(attributes, "title"),
		"o":            organization,
		"sn":           {name["familyName"].(string)},
		"sshPublicKey": sshPublicKey,
		"mail":         s,
		// Address
		"street":     getOptionalAttribute(address, "streetAddress"),
		"l":          getOptionalAttribute(address, "locality"),
		"postalCode": getOptionalAttribute(address, "postalCode"),
		"c":          getOptionalAttribute(address, "country"),
		"st":         getOptionalAttribute(address, "region"),
	}

	givenName, ok := name["givenName"]

	// Can be empty for community users
	if ok && givenName != "" {
		replaces["givenName"] = []string{givenName.(string)}
	}

	if telephones, ok := attributes["phoneNumbers"]; ok {
		for _, phone := range telephones.([]interface{}) {
			phoneEntry := phone.(map[string]interface{})
			phoneNr, ok := phoneEntry["value"].(string)
			if !ok {
				continue
			}
			phoneType, ok := phoneEntry["type"].(string)
			if !ok {
				continue
			}
			switch phoneType {
			case "work":
				replaces["telephoneNumber"] = []string{phoneNr}
				break
			case "mobile":
				replaces["mobile"] = []string{phoneNr}
				break
			}
		}
	}
	return replaces
}

func (h UserHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	return scim.Resource{}, errors.ScimError{Status: http.StatusNotImplemented, Detail: "replace is not implemented for users"}
}

func scimMailToLdap(attributes scim.ResourceAttributes) []string {
	mails, ok := attributes["emails"].([]interface{})

	if !ok {
		return []string{}
	}

	result := make([]string, 0, len(mails))

	primary := ""

	for _, entry := range mails {
		mail := entry.(map[string]interface{})

		if mail["primary"].(bool) {
			primary = mail["value"].(string)
			continue
		}

		result = append(result, mail["value"].(string))
	}

	if primary != "" {
		result = slices.Insert(result, 0, primary)
	}

	return result
}

func getOptionalAttributeSlice[T any](attributes scim.ResourceAttributes, name string, fallback []T) []T {
	value, ok := attributes[name]

	if !ok {
		return fallback
	}

	list := value.([]interface{})

	result := make([]T, 0, len(list))

	for _, entry := range list {
		result = append(result, entry.(T))
	}

	return result
}

func getOptionalAttribute(attributes scim.ResourceAttributes, name string) []string {
	value, ok := attributes[name]

	if !ok {
		return []string{}
	}

	return []string{value.(string)}
}

func ldapMailToSCIMMail(entry *ldap.Entry) []interface{} {
	ldapMails := entry.GetAttributeValues("mail")
	result := make([]interface{}, 0, len(ldapMails))
	for i, mail := range ldapMails {
		result = append(result, map[string]interface{}{
			"value":   mail,
			"type":    "work",
			"primary": i == 0,
		})
	}
	return result
}

func ldapEntryToUserResource(entry *ldap.Entry) scim.Resource {

	name := map[string]interface{}{
		"familyName": entry.GetAttributeValue("sn"),
		"givenName":  entry.GetAttributeValue("givenName"),
		"formatted":  entry.GetAttributeValue("cn"),
	}

	active, err := strconv.ParseBool(entry.GetAttributeValue("isActive"))

	if err != nil {
		slog.Warn("Error parsing active value", "value", entry.GetAttributeValue("isActive"))
		return scim.Resource{}
	}

	attributes := map[string]interface{}{
		"userName": entry.GetAttributeValue("uid"),
		"name":     name,
		"emails":   ldapMailToSCIMMail(entry),
		"active":   active,
	}

	address := ldapToAddress(entry)

	if len(address) > 0 {
		attributes["addresses"] = []map[string]interface{}{address}
	}

	phoneNumbers := ldapToPhones(entry)

	if len(phoneNumbers) > 0 {
		attributes["phoneNumbers"] = phoneNumbers
	}

	sshPubKeys := entry.GetAttributeValues("sshPublicKey")
	if len(sshPubKeys) > 0 {
		attributes["urn:ietf:params:scim:schemas:extension:suse:2.0:User"] = map[string]interface{}{
			"sshPublicKey": sshPubKeys,
		}
	}

	organization := entry.GetAttributeValues("o")
	if len(organization) == 1 { // single value
		attributes["urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"] = map[string]interface{}{
			"organization": organization[0],
		}
	}

	optionalAttributeToResource(entry, attributes, "title", "title")

	return scim.Resource{
		// This is the ID that will then be used to manage group memberships.
		// Since memberUid fields should contain the uid we return the uid here instead of the uuid.
		// This saves us from doing more mapping in the group handler
		// that would require resolving the uuid to the uid and vice versa on each request.
		ID:         entry.GetAttributeValue("uid"),
		ExternalID: optional.NewString(entry.DN),
		Attributes: attributes,
	}
}

func ldapToPhones(entry *ldap.Entry) []map[string]interface{} {
	var phoneNumbers []map[string]interface{}

	if mobile, ok := optionalAttribute(entry, "mobile"); ok {
		phoneNumbers = append(phoneNumbers, map[string]interface{}{
			"value": mobile,
			"type":  "mobile",
		})
	}

	if workPhone, ok := optionalAttribute(entry, "telephoneNumber"); ok {
		phoneNumbers = append(phoneNumbers, map[string]interface{}{
			"value": workPhone,
			"type":  "work",
		})
	}
	return phoneNumbers
}

func ldapToAddress(entry *ldap.Entry) map[string]interface{} {
	address := map[string]interface{}{}

	if street, ok := optionalAttribute(entry, "street"); ok {
		address["streetAddress"] = street
	}

	if postalCode, ok := optionalAttribute(entry, "postalCode"); ok {
		address["postalCode"] = postalCode
	}

	if locality, ok := optionalAttribute(entry, "l"); ok {
		address["locality"] = locality
	}

	if country, ok := optionalAttribute(entry, "c"); ok {
		address["country"] = country
	}

	if region, ok := optionalAttribute(entry, "st"); ok {
		address["region"] = region
	}
	return address
}

func optionalAttribute(entry *ldap.Entry, attribute string) (string, bool) {
	values := entry.GetAttributeValues(attribute)
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func optionalAttributeToResource(entry *ldap.Entry, attributes map[string]interface{}, attribute string, target string) {
	values := entry.GetAttributeValues(attribute)
	if len(values) == 0 {
		return
	}
	attributes[target] = &values[0]
}
