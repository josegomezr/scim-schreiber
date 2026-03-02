package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/go-ldap/ldap/v3"
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

	entries, err := ldapCtx.searchGroups(attributes["displayName"].(string), "cn")

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

	// TODO: Id's need something more qualifies, maybe we can use the UUID
	// generated in the IDP
	//
	// If not we should fallback to DN's everywhere.
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

	return nil
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

	entries, err := ldapCtx.searchGroups(id, "cn")

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

	err := params.FilterValidator.Validate()
	if err != nil {
		return scim.Page{}, err
	}

	groups, err := ldapCtx.searchGroups("*", "cn")

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
			err = params.FilterValidator.PassesFilter(resource.Attributes)
			if err != nil {
				slog.Info("An error occurred while validating filter", "err", err)
				continue
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

func (h GroupHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH group", "id", id)

	ldapCtx, ok := GetLDAPContext(r.Context())

	if !ok {
		slog.Warn("Failed to get LDAP context")
		return scim.Resource{}, errors.ScimError{
			Status: http.StatusInternalServerError,
		}
	}

	entries, err := ldapCtx.searchGroups(id, "cn")

	if err != nil {
		slog.Error("An error occurred while getting group", "id", id, "err", err)
		return scim.Resource{}, errors.ScimErrorInternal
	}

	for entry, err := range entries {
		if err != nil {
			slog.Error("An error occurred while getting group", "id", id, "err", err)
			return scim.Resource{}, errors.ScimErrorInternal
		}

		adds, removes, replaces := classifyPatchOperations(operations)
		h.transformMemberUUIDtoUID(ldapCtx, adds, "members", "memberUid")
		h.transformMemberUUIDtoUID(ldapCtx, removes, "members", "memberUid")

		err := ldapCtx.UpdateEntry(entry.DN, adds, removes, replaces)

		if err != nil {
			slog.Error("An error occurred while getting group", "id", id, "err", err)
			return scim.Resource{}, errors.ScimErrorInternal
		}

		updatedEntry, err := ldapCtx.getByDN(entry.DN)

		if err != nil {
			slog.Error("An error occurred while getting group", "id", id, "err", err)
			return scim.Resource{}, errors.ScimErrorInternal
		}

		return ldapEntryToGroupResource(updatedEntry), nil
	}

	return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
}

func (h GroupHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	return scim.Resource{}, errors.ScimError{Status: http.StatusNotImplemented}
}

func (h *GroupHandler) transformMemberUUIDtoUID(ldapCtx *LdapUtil, attrmap map[string][]string, source string, dest string) {
	if attrmap[source] == nil {
		return
	}

	if attrmap[dest] == nil {
		attrmap[dest] = make([]string, 0)
	}

	for _, userUuid := range attrmap[source] {
		if lookup := ldapCtx.searchUserByUUID(userUuid); lookup != nil {
			attrmap[dest] = append(attrmap[dest], lookup.GetAttributeValue("uid"))
		}
	}

	delete(attrmap, source)

}

func classifyPatchOperations(operations []scim.PatchOperation) (map[string][]string, map[string][]string, map[string][]string) {
	adds := make(map[string][]string)
	removes := make(map[string][]string)
	replaces := make(map[string][]string)

	for _, op := range operations {
		if reflect.ValueOf(op.Value).Kind() == reflect.Slice {
			for _, singleVal := range CastMultiValue[map[string]interface{}](op.Value) {
				value := CastSingleValue[string](singleVal["value"])

				switch op.Op {
				case scim.PatchOperationAdd:
					adds[op.Path.String()] = append(adds[op.Path.String()], value)
				case scim.PatchOperationRemove:
					removes[op.Path.String()] = append(removes[op.Path.String()], value)
				default:
					slog.Info("unknown OP", "operation", op.Op, "path", op.Path, "value", singleVal)
					continue
				}
			}
		} else {
			switch op.Op {
			case scim.PatchOperationAdd:
				adds[op.Path.String()] = append(adds[op.Path.String()], fmt.Sprintf("%s", op.Value))
			case scim.PatchOperationRemove:
				removes[op.Path.String()] = append(removes[op.Path.String()], fmt.Sprintf("%s", op.Value))
			case scim.PatchOperationReplace:
				for k, v := range op.Value.(map[string]interface{}) {
					replaces[k] = append(replaces[k], fmt.Sprintf("%s", v))
				}
			default:
				slog.Info("unknown OP", "operation", op.Op, "path", op.Path, "value", op.Value)
				continue
			}
		}
	}

	return adds, removes, replaces
}
