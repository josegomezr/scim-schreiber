package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/filter"
	"github.com/elimity-com/scim/optional"
	"github.com/josegomezr/scim-schreiber-ldap/internal/casting"
	"github.com/josegomezr/scim-schreiber-ldap/internal/jira-server"
	scim_filter_parser "github.com/scim2/filter-parser/v2"
)

type Exclusions map[string]interface{}
type GroupExclusions map[string]Exclusions

type GroupHandler struct {
	cfg             *Config
	client          *jira.Client
	groupExclusions GroupExclusions
}

func (h GroupHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Groups", "request", attributes)

	groupRequest := resourceToJiraGroup(attributes)
	group, err := h.client.CreateGroup(*groupRequest)
	if err != nil {
		if errors.Is(err, jira.GroupAlreadyExists) {
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusConflict, Detail: err.Error()}
		}
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return jiraGroupToGroupResource(group), nil
}

func (h GroupHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE /v2/Groups", "id", id)
	err := h.client.DeleteGroup(id)
	if err != nil {
		return scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

func (h GroupHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Groups", "id", id)

	if id == "" {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound("")
	}

	jiraGroup, err := h.client.GetGroup(id)
	if err != nil {
		return scim.Resource{}, err
	}

	if jiraGroup == nil {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
	}

	return jiraGroupToGroupResource(jiraGroup), nil
}

func displayNameFromFilter(filterValidator *filter.Validator) (string, error) {
	if filterValidator == nil {
		return "", nil
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
	return f.CompareValue.(string), nil
}

func jiraGroupToGroupResource(entry *jira.Group) scim.Resource {
	members := []map[string]string{}
	for _, mem := range entry.Members {
		memberMap := make(map[string]string)
		memberMap["value"] = mem.UserName
		memberMap["display"] = mem.DisplayName
		members = append(members, memberMap)
	}

	return scim.Resource{
		ID:         entry.DisplayName,
		ExternalID: optional.NewString(entry.DisplayName),
		Attributes: map[string]interface{}{
			"rawGroupLocation": entry.Self,
			"displayName":      entry.DisplayName,
			"members":          members,
		},
	}
}

func resourceToJiraGroup(resourceAttrs map[string]interface{}) *jira.Group {
	displayName := casting.SingleValue[string](resourceAttrs["displayName"])
	return &jira.Group{
		DisplayName: displayName,
	}
}

func (h GroupHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Groups", "params", params)

	principal, err := displayNameFromFilter(params.FilterValidator)
	if err != nil {
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)
	for jiraGroup, err := range h.client.ListAllGroups(principal) {
		if err != nil {
			return scim.Page{}, err
		}
		resources = append(resources, jiraGroupToGroupResource(jiraGroup))
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func (h GroupHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Groups", "id", id, "operations", operations)

	adds := []string{}
	removes := []string{}

	exclusions := Exclusions{}
	if exc, ok := h.groupExclusions[id]; ok {
		exclusions = exc
	}

	// TODO(josegomezr): Align the duplicated code here
	for _, op := range operations {
		if op.Path == nil {
			body := casting.SingleValue[map[string]interface{}](op.Value)
			for _, singleVal := range casting.MultiValue[map[string]interface{}](body["members"]) {
				value := casting.SingleValue[string](singleVal["value"])
				if _, ok := exclusions[value]; ok {
					slog.Info("Member present in exclusion list, ignoring change", "user", value, "group", id)
					continue
				}
			}
			continue
		}

		switch op.Path.String() {
		case "members":
			for _, singleVal := range casting.MultiValue[map[string]interface{}](op.Value) {
				value := casting.SingleValue[string](singleVal["value"])
				if _, ok := exclusions[value]; ok {
					slog.Info("Member present in exclusion list, ignoring change", "user", value, "group", id)
					continue
				}

				switch op.Op {
				case scim.PatchOperationAdd:
					adds = append(adds, value)
				case scim.PatchOperationRemove:
					removes = append(removes, value)
				default:
					return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Only membership add/remove is allowed"}
				}
			}
		default:
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Only membership changes are allowed"}
		}
	}

	slog.Info("Processing patch", "adds", adds, "removes", removes)
	if len(adds) == 0 && len(removes) == 0 {
		return scim.Resource{}, nil
	}

	// JIRA Server doesn't have single membership endpoint, so we gotta build a
	// map of memberships beforehand to be able to optimize calls and avoid full
	// group scan for every operation below.
	prexistingUsers := map[string]bool{}
	members, err := h.client.GetGroupMembers(id)
	if err != nil {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Could not fetch memberships"}
	}
	for _, user := range members {
		prexistingUsers[user.UserName] = true
		prexistingUsers[user.EmailAddress] = true
	}

	// members are guaranteed to be multivalued
	var pushErrors string
	for _, value := range adds {
		if _, ok := prexistingUsers[value]; ok {
			slog.Info("User is part of the group. Skipping operation", "operation", "add", "group", id, "user", value, "error", err)
			continue
		}
		if err := h.client.AddUserToGroup(value, id); err != nil {
			pushErrors += err.Error() + "\n"
			slog.Warn("Error adding user from group", "group", id, "user", value, "error", err)
		}
	}
	for _, value := range removes {
		if _, ok := prexistingUsers[value]; !ok {
			slog.Info("User is not part of the group. Skipping operation", "operation", "remove", "group", id, "user", value, "error", err)
			continue
		}
		if err := h.client.RemoveUserFromGroup(value, id); err != nil {
			pushErrors += err.Error() + "\n"
			slog.Warn("Error removing user from group", "group", id, "user", value, "error", err)
		}
	}

	if !h.cfg.IgnoreGroupAddResponseCode && len(pushErrors) > 0 {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: pushErrors}
	}
	return scim.Resource{}, nil
}

func (h GroupHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Groups", "id", id, "attributes", attributes)

	groupRequest := resourceToJiraGroup(attributes)
	jiraGroup, err := h.client.UpdateGroup(id, *groupRequest)
	if err != nil {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}
	return jiraGroupToGroupResource(jiraGroup), nil
}
