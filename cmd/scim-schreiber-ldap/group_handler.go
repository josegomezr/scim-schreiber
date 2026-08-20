package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/filter"
	"github.com/elimity-com/scim/optional"
	"github.com/go-ldap/ldap/v3"
	scim_filter_parser "github.com/scim2/filter-parser/v2"

	"github.com/josegomezr/scim-schreiber-ldap/internal/model"
)

type GroupHandler struct {
	cfg *Config
}

// TODO(josegomezr): Groups don't self-heal after an 409 Conflict like
//
//					     users, we gotta make it behave like an UPSERT rather
//	                  than a POST+GET (as users do).
//	                  Prolly make sense to make group & users behave like an UPSERT
func (h GroupHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {

	slog.Info("Creating group", "request", attributes)

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	entries, err := ldapCtx.searchGroups(ldap.EscapeFilter(attributes["displayName"].(string)), "cn")

	if err != nil {
		return scim.Resource{}, errors.ScimErrorInternal
	}

	for entry, err := range entries {
		if err != nil {
			return scim.Resource{}, errors.ScimErrorInternal
		}

		if h.cfg.GroupCreationIsUpsert {
			return ldapEntryToGroupResource(entry), nil
		}

		return scim.Resource{}, errors.ScimErrorUniqueness
	}

	return scim.Resource{}, errors.ScimError{Status: http.StatusForbidden}
}

func ldapEntryToGroupResource(entry *ldap.Entry) scim.Resource {
	members := []model.ValueObj{}
	for _, value := range entry.GetAttributeValues("memberUid") {
		members = append(members, model.ValueObj{
			Value: value,
		})
	}

	// Display Name must be unique since that is what Authentik uses as a primary key to search for groups.
	return scim.Resource{
		ID:         entry.GetAttributeValue("cn"),
		ExternalID: optional.NewString(entry.DN),
		Attributes: map[string]interface{}{
			"displayName": entry.GetAttributeValue("cn"),
			"members":     members,
		},
	}
}

// TODO(josegomezr): Sometimes IDP's don't really _delete_ things. We gotta find a creative way to
//
//	                  issue a deletion from SCIM.
//						 Model is set to be deleted, task to roll updates is scheduled in the background
//	                  by the time the task is executed, the record does not exist in the DB anymore.
func (h GroupHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE group", "id", id)

	// TODO Implement

	return errors.ScimError{Status: http.StatusNotImplemented, Detail: "Delete is not implemented"}
}

func (h GroupHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET group", "id", id)

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	entries, err := ldapCtx.searchGroups(ldap.EscapeFilter(id), "cn")

	if err != nil {
		slog.Error("An error occurred while getting group", "id", id, "err", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	for entry, err := range entries {
		if err != nil {
			slog.Error("An error occurred while getting group", "id", id, "err", err)
			return scim.Resource{}, errors.ScimErrorInternal
		}

		return ldapEntryToGroupResource(entry), nil
	}

	return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
}

func displayNameFromFilter(filterValidator *filter.Validator) (string, error) {
	if filterValidator == nil {
		return "*", nil
	}
	f, ok := filterValidator.GetFilter().(*scim_filter_parser.AttributeExpression)
	if !ok {
		return "", fmt.Errorf("only single expressions are supported")
	}
	if f.Operator != "eq" {
		return "", fmt.Errorf("only operator 'eq' is supported in filters")
	}
	if f.AttributePath.AttributeName != "displayName" {
		return "", fmt.Errorf("only 'displayName' is supported in filters")
	}
	return ldap.EscapeFilter(f.CompareValue.(string)), nil
}

func (h GroupHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Page{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	if params.Count == 0 {
		groupCount, err := ldapCtx.CountGroups()

		if err != nil {
			return scim.Page{}, errors.ScimErrorInternal
		}

		return scim.Page{
			TotalResults: groupCount,
		}, nil
	}

	resources := make([]scim.Resource, 0)

	dnFilter, err := displayNameFromFilter(params.FilterValidator)

	if err != nil {
		return scim.Page{}, errors.ScimErrorInternal
	}

	groups, err := ldapCtx.searchGroups(dnFilter, "cn")

	if err != nil {
		return scim.Page{}, errors.ScimErrorInternal
	}

	i := 1

	for entry := range groups {
		// Ldap pagination does not support start index. So skip until we find the correct entry
		if i > (params.StartIndex + params.Count - 1) {
			// If we wanted to provide the correct result in TotalResults we'd actually have to keep counting here.
			break
		}

		if i >= params.StartIndex {
			resource := ldapEntryToGroupResource(entry)
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

func (h GroupHandler) scimToLdapAttributes(attributes map[string][]string) map[string][]string {
	result := make(map[string][]string)

	for attribute, value := range attributes {
		switch attribute {
		case "members.value":
			result["memberUid"] = value
			break
		}
	}

	return result
}

func (h GroupHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH group", "id", id)

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	entry, err := ldapCtx.GetGroup(id)

	if err != nil {
		slog.Error("An error occurred while getting group", "id", id, "err", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	if entry == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	adds, removes, replaces := classifyPatchOperations(operations)

	adds = h.scimToLdapAttributes(adds)
	removes = h.scimToLdapAttributes(removes)
	replaces = h.scimToLdapAttributes(replaces)

	err = ldapCtx.UpdateEntry(entry.DN, adds, removes, replaces)

	if err != nil {
		slog.Error("An error occurred while updating group", "id", id, "err", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	// Read the updated group
	updatedEntry, err := ldapCtx.GetGroup(id)

	if err != nil {
		slog.Error("An error occurred while getting group", "id", id, "err", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	return ldapEntryToGroupResource(updatedEntry), nil
}

func (h GroupHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	return scim.Resource{}, errors.ScimError{Status: http.StatusNotImplemented}
}

func convertToLdap(input interface{}, path string, output map[string][]string) {
	switch input.(type) {
	case map[string]interface{}:
		for key, value := range input.(map[string]interface{}) {
			prefix := path
			if prefix != "" {
				prefix += "."
			}
			convertToLdap(value, prefix+key, output)
		}
		break
	case string:
		if output[path] == nil {
			output[path] = make([]string, 0)
		}

		output[path] = append(output[path], input.(string))
		break
	case []interface{}:
		for _, value := range input.([]interface{}) {
			convertToLdap(value, path, output)
		}
		break
	default:
		slog.Warn("Unknown patch value type", "value", input)
	}
}

func classifyPatchOperations(operations []scim.PatchOperation) (map[string][]string, map[string][]string, map[string][]string) {
	adds := make(map[string][]string)
	removes := make(map[string][]string)
	replaces := make(map[string][]string)

	for _, op := range operations {
		path := ""
		if op.Path != nil {
			path = op.Path.String()
		}

		switch op.Op {
		case scim.PatchOperationAdd:
			convertToLdap(op.Value, path, adds)
			break
		case scim.PatchOperationRemove:
			convertToLdap(op.Value, path, removes)
			break
		case scim.PatchOperationReplace:
			convertToLdap(op.Value, path, replaces)
			break
		}

	}

	return adds, removes, replaces
}
