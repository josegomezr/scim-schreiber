package main

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/go-ldap/ldap/v3"
)

type UserHandler struct {
	cfg *Config
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

	if ldapCtx.searchUser(username) != nil {
		return scim.Resource{}, errors.ScimErrorUniqueness
	}

	if !h.cfg.AllowUserCreation {
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusForbidden,
		}
	}

	return scim.Resource{}, errors.ScimError{
		Status: http.StatusNotImplemented,
	}
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

	if u := ldapCtx.searchUserByUUID(id); u != nil {
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

	entry := ldapCtx.searchUserByUUID(id)

	if entry == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	return ldapEntryToUserResource(entry), nil
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

	if params.FilterValidator != nil {
		err := params.FilterValidator.Validate()
		if err != nil {
			return scim.Page{}, err
		}
	}

	users, err := ldapCtx.searchUsers("*", "uid")

	if err != nil {
		return scim.Page{}, errors.ScimErrorInternal
	}

	i := 1

	for entry := range users {
		// Ldap pagination does not support start index. So skip until we find the correct entry
		if i > (params.StartIndex + params.Count - 1) {
			// If we wanted to provide the correct result in TotalResults we'd actually have to keep counting here.
			break
		}

		if i >= params.StartIndex {
			resource := ldapEntryToUserResource(entry)
			if params.FilterValidator != nil {
				err = params.FilterValidator.PassesFilter(resource.Attributes)
				if err != nil {
					slog.Info("An error occurred while validating filter", "err", err)
					continue
				}
			}
			resources = append(resources, resource)
		}
		i++
	}

	return scim.Page{
		TotalResults: i,
		Resources:    resources,
	}, nil
}

func (h UserHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	return scim.Resource{}, errors.ScimError{Status: http.StatusNotImplemented, Detail: "Patch is not implemented for users"}
}

func (h UserHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	entry := ldapCtx.searchUserByUUID(id)
	if entry == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	slog.Info("Found user", "entry", entry)

	// TODO(josegomezr): change more details
	slog.Info("Updating user details.", "from", entry.GetAttributeValue("cn"), "to", attributes["displayName"])

	s := scimMailToLdap(attributes)

	replaces := map[string][]string{
		"cn":           {attributes["displayName"].(string)},
		"sshPublicKey": getOptionalAttribute(attributes, "sshPublicKey", []string{}),
		"mail":         s,
	}

	err := ldapCtx.UpdateEntry(entry.DN, nil, nil, replaces)

	if err != nil {
		slog.Error("Error updating entry", "error", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	// Get updated entry
	entry = ldapCtx.searchUserByUUID(id)
	// return resource with replaced attributes
	return ldapEntryToUserResource(entry), nil
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

func getOptionalAttribute[T any](attributes scim.ResourceAttributes, name string, fallback T) T {
	value, ok := attributes[name]

	if !ok {
		return fallback
	}

	return value.(T)
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

	return scim.Resource{
		ID:         entry.GetAttributeValue("uuid"),
		ExternalID: optional.NewString(entry.DN),
		Attributes: map[string]interface{}{
			"userName":    entry.GetAttributeValue("uid"),
			"displayName": entry.GetAttributeValue("cn"),
			"emails":      ldapMailToSCIMMail(entry),
		},
	}
}
