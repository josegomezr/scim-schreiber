package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/josegomezr/scim-schreiber-ldap/internal/server"
	"github.com/scim2/filter-parser/v2"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/licensing/v1"

	"github.com/josegomezr/scim-schreiber-ldap/internal/casting"
	"github.com/josegomezr/scim-schreiber-ldap/internal/model"
	"github.com/josegomezr/scim-schreiber-ldap/internal/utils"
)

type UserHandler struct {
	cfg                *Config
	adminClient        *admin.Service
	licenseClient      *licensing.Service
	productInformation *ProductInformation
}

func (h UserHandler) Create(_ *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Users", "request", attributes)

	userRequest, err := h.resourceToUser(attributes)

	if err != nil {
		return scim.Resource{}, err
	}

	// Google forces us to set a password...
	userRequest.Password = rand.Text()

	user, err := h.adminClient.Users.Insert(userRequest).Do()
	if err != nil {
		var googleErr *googleapi.Error
		if errors.As(err, &googleErr) && googleErr.Code == http.StatusConflict {
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusConflict, Detail: googleErr.Error()}
		}
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	wantLicenses, err := h.updateLicenses(user, attributes)
	if err != nil {
		return scim.Resource{}, err
	}

	err = h.updateAliases(user, attributes)
	if err != nil {
		return scim.Resource{}, err
	}

	resource := userToUserResource(user)
	resource.Attributes["entitlements"] = wantLicenses
	return resource, nil
}

func (h UserHandler) Delete(_ *http.Request, id string) error {
	slog.Info("DELETE /v2/Users", "id", id)
	err := h.adminClient.Users.Delete(id).Do()
	if err != nil {
		return scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

// Get the user.
// id: Identifies the user in the API request. The value can be the
// user's primary email address, alias email address, or unique user ID.
func (h UserHandler) Get(_ *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Users", "id", id)

	if id == "" {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound("")
	}

	user, err := h.adminClient.Users.Get(id).Projection("full").Do()
	if err != nil {
		slog.Warn("Error getting user", "id", id, "error", err)
		return scim.Resource{}, err
	}

	if user == nil {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
	}
	resource := userToUserResource(user)

	licensesForUser, err := h.getLicenses(user)

	if err != nil {
		return scim.Resource{}, err
	}

	resource.Attributes["entitlements"] = h.licenseToResource(licensesForUser)

	return resource, nil
}

func (h UserHandler) getLicenses(user *admin.User) ([]Product, error) {
	licensesForUser := make([]Product, 0)

	for _, product := range h.productInformation.Products {
		_, err := h.licenseClient.LicenseAssignments.Get(product.ProductId, product.SkuId, user.PrimaryEmail).Do()

		if err != nil {
			var googleErr *googleapi.Error
			if errors.As(err, &googleErr) && googleErr.Code == http.StatusNotFound {
				continue
			}

			return nil, err
		}

		licensesForUser = append(licensesForUser, product)
	}
	return licensesForUser, nil
}

func (h UserHandler) licenseToResource(licenses []Product) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(licenses))
	for _, assignment := range licenses {
		out = append(out, map[string]interface{}{
			"value": h.productInformation.ReverseProductMap[assignment.ProductId][assignment.SkuId],
			"type":  "license",
		})
	}
	return out
}

func emailToResource(entry *admin.User) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(entry.Aliases)+1)

	out = append(out, map[string]interface{}{
		"primary": true,
		"value":   entry.PrimaryEmail,
		"type":    "work",
	})

	for _, alias := range entry.Aliases {
		out = append(out, map[string]interface{}{
			"primary": false,
			"value":   alias,
			"type":    "work",
		})
	}
	return out
}

func googleToAddress(entry map[string]interface{}) map[string]interface{} {
	address := map[string]interface{}{
		"streetAddress": entry["streetAddress"],
		"postalCode":    entry["postalCode"],
		"locality":      entry["locality"],
		"country":       entry["countryCode"],
		"region":        entry["region"],
		"type":          entry["type"],
	}

	return address
}

func userToUserResource(entry *admin.User) scim.Resource {

	enterpriseExt := map[string]interface{}{}

	title := ""

	if entry.Organizations != nil {
		organizations := entry.Organizations.([]interface{})
		for _, orgRaw := range organizations {
			org := orgRaw.(map[string]interface{})

			if org["customType"] == "work" {
				enterpriseExt = map[string]interface{}{
					"organization": casting.SingleValue[string](org["name"]),
					"costCenter":   casting.SingleValue[string](org["costCenter"]),
					"department":   casting.SingleValue[string](org["department"]),
				}
				title = org["title"].(string)
				break
			}

		}
	}

	if entry.ExternalIds != nil {
		externalIds := entry.ExternalIds.([]interface{})
		for _, externalId := range externalIds {
			id := externalId.(map[string]interface{})

			if id["type"] == "organization" {
				enterpriseExt["employeeNumber"] = id["value"].(string)
			}
		}
	}

	addresses := make([]map[string]interface{}, 0)
	if entry.Addresses != nil {
		for _, addr := range entry.Addresses.([]interface{}) {

			addrCast := addr.(map[string]interface{})
			addresses = append(addresses, googleToAddress(addrCast))
		}
	}

	phones := make([]map[string]interface{}, 0)
	if entry.Phones != nil {
		for _, addr := range entry.Phones.([]interface{}) {

			phoneCast := addr.(map[string]interface{})

			phoneType := ""

			switch phoneCast["type"] {
			case "work":
				phoneType = "work"
			case "work_mobile":
				phoneType = "mobile"
			default:
				continue
			}

			phones = append(phones, map[string]interface{}{
				"type":  phoneType,
				"value": phoneCast["value"].(string),
			})
		}
	}

	customFields := UnmarshallCustomSchemas(entry.CustomSchemas)

	googleExt := map[string]interface{}{
		"orgUnitPath": entry.OrgUnitPath,
		"relations":   entry.Relations,
	}

	UpdateSCIMExtensions(customFields, googleExt, enterpriseExt)

	return scim.Resource{
		ID:         entry.Id,
		ExternalID: optional.NewString(entry.PrimaryEmail),
		Attributes: map[string]interface{}{
			"name": map[string]interface{}{
				"familyName": entry.Name.FamilyName,
				"givenName":  entry.Name.GivenName,
				"formatted":  entry.Name.FullName,
			},
			"emails":       emailToResource(entry),
			"displayName":  entry.Name.DisplayName,
			"userName":     entry.PrimaryEmail,
			"active":       !entry.Suspended,
			"addresses":    addresses,
			"title":        title,
			"phoneNumbers": phones,
			"userType":     customFields.GetUserType(),
			"urn:ietf:params:scim:schemas:extension:suse:2.0:GoogleUser": googleExt,
			"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": enterpriseExt,
		},
	}
}

func (h UserHandler) resourceToUser(resourceAttrs map[string]interface{}) (*admin.User, error) {
	nameMap := casting.SingleValue[map[string]interface{}](resourceAttrs["name"])

	emails, ok := resourceAttrs["emails"].([]interface{})

	primaryEmail := ""
	if ok {
		for _, e := range emails {
			email, ok := e.(map[string]interface{})
			if ok && email["primary"].(bool) {
				primaryEmail = email["value"].(string)
			}
		}
	}

	if primaryEmail == "" {
		slog.Warn("Primary email not found")
		return nil, scimerrors.ScimErrorBadRequest("Need a primary email")
	}

	userName := casting.SingleValue[string](resourceAttrs["userName"])

	if primaryEmail != userName {
		slog.Warn("Need a primary email to match username")
		return nil, scimerrors.ScimErrorBadRequest("Need a primary email to match username")
	}

	orgUnitPath := ""
	if val, ok := utils.GetExtensionAttribute(resourceAttrs, SCHEMA_GOOGLE_USER, "orgUnitPath"); ok {
		orgUnitPath = casting.SingleValue[string](val)
	}
	googleAddresses := make([]admin.UserAddress, 0)

	if _, ok := resourceAttrs["addresses"]; ok {
		addresses := resourceAttrs["addresses"].([]interface{})
		googleAddresses = make([]admin.UserAddress, 0, len(addresses))
		for _, addressRaw := range addresses {
			address := addressRaw.(map[string]interface{})

			userAddress := admin.UserAddress{
				CountryCode:   utils.GetOptionalSingleAttribute(address, "country"),
				Locality:      utils.GetOptionalSingleAttribute(address, "locality"),
				PostalCode:    utils.GetOptionalSingleAttribute(address, "postalCode"),
				Region:        utils.GetOptionalSingleAttribute(address, "region"),
				StreetAddress: utils.GetOptionalSingleAttribute(address, "streetAddress"),
				Type:          utils.GetOptionalSingleAttribute(address, "type"),
			}

			userAddress.Formatted = strings.Join([]string{
				userAddress.StreetAddress, userAddress.Locality,
				userAddress.PostalCode, userAddress.Region, userAddress.CountryCode,
			}, ", ")

			googleAddresses = append(googleAddresses, userAddress)
		}
	}

	relations := make([]admin.UserRelation, 0)
	if val, ok := utils.GetExtensionAttribute(resourceAttrs, SCHEMA_GOOGLE_USER, "relations"); ok {
		relationsRaw := val.([]interface{})

		for _, relationRaw := range relationsRaw {
			relation := relationRaw.(map[string]interface{})
			relations = append(relations, admin.UserRelation{
				Type:  relation["type"].(string),
				Value: relation["value"].(string),
			})
		}
	}

	organizations := make([]admin.UserOrganization, 0, 1)
	if val, ok := utils.GetExtensionAttribute(resourceAttrs, model.SCHEMA_ENTERPRISE_USER, "organization"); ok {
		organization := casting.SingleValue[string](val)

		costCenter := ""
		if val, ok := utils.GetExtensionAttribute(resourceAttrs, model.SCHEMA_ENTERPRISE_USER, "costCenter"); ok {
			costCenter = casting.SingleValue[string](val)
		}

		department := ""
		if val, ok := utils.GetExtensionAttribute(resourceAttrs, model.SCHEMA_ENTERPRISE_USER, "department"); ok {
			department = casting.SingleValue[string](val)
		}

		organizations = append(organizations, admin.UserOrganization{
			CostCenter: costCenter,
			CustomType: "work",
			Department: department,
			Name:       organization,
			Title:      utils.GetOptionalSingleAttribute(resourceAttrs, "title"),
		})
	}

	phones := utils.GetPhones(resourceAttrs)

	googlePhones := make([]admin.UserPhone, 0, 2)

	if phones.Mobile != "" {
		googlePhones = append(googlePhones, admin.UserPhone{
			Type:  "work_mobile",
			Value: phones.Mobile,
		})
	}

	if phones.Work != "" {
		googlePhones = append(googlePhones, admin.UserPhone{
			Type:  "work",
			Value: phones.Work,
		})
	}

	var externalIds []admin.UserExternalId

	if val, ok := utils.GetExtensionAttribute(resourceAttrs, model.SCHEMA_ENTERPRISE_USER, "employeeNumber"); ok {
		externalIds = []admin.UserExternalId{
			{
				Type:  "organization",
				Value: casting.SingleValue[string](val),
			},
		}
	}

	bytes, err := CustomResourceToUser(resourceAttrs, googleAddresses, h.getAliases(resourceAttrs))

	if err != nil {
		return nil, err
	}

	return &admin.User{
		// Only update primary e-mails. Legacy aliases are managed in Google Workspace
		PrimaryEmail: userName,
		Name: &admin.UserName{
			DisplayName: casting.SingleValue[string](resourceAttrs["displayName"]),
			FamilyName:  casting.SingleValue[string](nameMap["familyName"]),
			GivenName:   casting.SingleValue[string](nameMap["givenName"]),
			FullName:    casting.SingleValue[string](nameMap["formatted"]),
		},
		OrgUnitPath:   orgUnitPath,
		Suspended:     !casting.SingleValue[bool](resourceAttrs["active"]),
		CustomSchemas: bytes,
		Addresses:     googleAddresses,
		Relations:     relations,
		Organizations: organizations,
		Phones:        googlePhones,
		ExternalIds:   externalIds,
		Archived:      !casting.SingleValue[bool](resourceAttrs["active"]),
		//Emails:        googleEmails,

		ForceSendFields: []string{
			"Archived", "Suspended",
		},
	}, nil
}

func (h UserHandler) GetAll(httpRequest *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Users", "params", params)
	principal, err := model.PrincipalFromFilter(params.FilterValidator)
	if err != nil {
		slog.Info("Invalid filter", "err", err)
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)

	request := h.adminClient.Users.List().Domain(h.cfg.Domain).Projection("full")

	if principal != "" {
		request = request.Query("email=" + principal)
	}

	pageToken := httpRequest.URL.Query().Get("pageToken")

	if pageToken != "" {
		request = request.PageToken(pageToken)
	}

	// Pagination is not implemented here. But we also don't really need it.
	users, err := request.Do()

	if err != nil {
		return scim.Page{}, err
	}

	for _, user := range users.Users {
		resources = append(resources, userToUserResource(user))
	}

	if users.NextPageToken != "" {
		server.SetResponseHeader(httpRequest.Context(), "pageToken", users.NextPageToken)
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func isEmptyPath(path *filter.Path) bool {
	return path == nil || path.String() == ""
}

func (h UserHandler) Patch(_ *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Users", "id", id, "operations", operations)

	if len(operations) != 1 {
		slog.Warn("Only one replace is allowed")
		return scim.Resource{}, scimerrors.ScimErrorBadRequest("Only one replace is allowed")
	}

	operation := operations[0]

	if operation.Op != scim.PatchOperationReplace || !isEmptyPath(operation.Path) {
		slog.Warn("Only full replace is allowed")
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Only full replace is allowed"}
	}

	attributes, ok := operation.Value.(map[string]interface{})

	if !ok {
		slog.Warn("Value must be a JSON object")
		return scim.Resource{}, scimerrors.ScimErrorBadRequest("Value must be a JSON object")
	}

	return h.updateUser(attributes, id)

}

func (h UserHandler) Replace(_ *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Users", "id", id, "attributes", attributes)

	return h.updateUser(attributes, id)
}

func (h UserHandler) updateUser(attributes scim.ResourceAttributes, id string) (scim.Resource, error) {
	userRequest, err := h.resourceToUser(attributes)

	if err != nil {
		return scim.Resource{}, err
	}

	user, err := h.adminClient.Users.Update(id, userRequest).Do()
	if err != nil {
		slog.Warn("Error updating user", "id", id, "err", err)
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	wantLicenses, err := h.updateLicenses(user, attributes)
	if err != nil {
		slog.Warn("Error updating licenses", "id", id, "err", err)
		return scim.Resource{}, err
	}

	err = h.updateAliases(user, attributes)
	if err != nil {
		return scim.Resource{}, err
	}

	resource := userToUserResource(user)
	resource.Attributes["entitlements"] = wantLicenses
	return resource, nil
}

func (h UserHandler) getAliases(attributes scim.ResourceAttributes) []string {
	emails, ok := attributes["emails"].([]interface{})

	if !ok {
		return nil
	}

	wantAliases := make([]string, 0, len(emails))
	for _, e := range emails {
		email, ok := e.(map[string]interface{})
		if ok {
			primary, ok := email["primary"]

			if !ok || primary.(bool) {
				continue
			}

			_, domain, found := strings.Cut(email["value"].(string), "@")

			if found && domain == h.cfg.Domain {
				wantAliases = append(wantAliases, email["value"].(string))
			} else {
				slog.Warn("Invalid alias address", "email", email["value"])
			}
		}
	}
	return wantAliases
}

func (h UserHandler) updateAliases(user *admin.User, attributes scim.ResourceAttributes) error {

	existingAliases := user.Aliases
	wantAliases := h.getAliases(attributes)

	for _, alias := range wantAliases {
		if slices.Contains(existingAliases, alias) {
			continue
		}

		_, err := h.adminClient.Users.Aliases.Insert(user.Id, &admin.Alias{
			Alias: alias,
		}).Do()

		if err != nil {
			return err
		}
	}

	for _, alias := range existingAliases {
		if slices.Contains(wantAliases, alias) {
			continue
		}

		err := h.adminClient.Users.Aliases.Delete(user.Id, alias).Do()

		if err != nil {
			return err
		}
	}

	// Update the user with the new state.
	user.Aliases = wantAliases

	return nil
}

func (h UserHandler) updateLicenses(user *admin.User, attributes scim.ResourceAttributes) ([]map[string]interface{}, error) {
	hasLicenses, err := h.getLicenses(user)

	if err != nil {
		return nil, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	wantLicenses := []Product{}
	// Remove licenses is user is marked as inactive
	if !user.Suspended {
		tmp, ok := attributes["entitlements"]
		if ok {
			tmpCast, ok := tmp.([]interface{})
			if ok {
				for _, element := range tmpCast {
					if license, ok := element.(map[string]interface{}); ok {
						if license["type"] == "license" {
							wantLicenses = append(wantLicenses, h.productInformation.Products[license["value"].(string)])
						}
					}
				}
			}
		}
	}

REMOVE:
	for _, l := range hasLicenses {
		for _, w := range wantLicenses {
			if l.ProductId == w.ProductId && l.SkuId == w.SkuId {
				continue REMOVE
			}
		}

		_, err := h.licenseClient.LicenseAssignments.Delete(l.ProductId, l.SkuId, user.PrimaryEmail).Do()
		if err != nil {
			var googleErr *googleapi.Error
			if errors.As(err, &googleErr) && googleErr.Code == http.StatusBadRequest {
				// This means that the license is auto-assigned and can't be removed manually.
				continue
			}
			return nil, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
		}

	}

ADD:
	for _, w := range wantLicenses {
		for _, l := range hasLicenses {
			if l.ProductId == w.ProductId && l.SkuId == w.SkuId {
				continue ADD
			}
		}

		license := licensing.LicenseAssignmentInsert{
			UserId: user.PrimaryEmail,
		}
		_, err := h.licenseClient.LicenseAssignments.Insert(w.ProductId, w.SkuId, &license).Do()
		if err != nil {
			return nil, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
		}
	}
	return h.licenseToResource(wantLicenses), nil
}
