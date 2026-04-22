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

type GroupHandler struct {
	cfg    *Config
	client *jira.Client
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
		memberMap["value"] = mem.Name
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

	for _, op := range operations {
		switch op.Path.String() {
		case "members":
			continue
		default:
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Only membership changes are allowed"}
		}
	}

	for _, op := range operations {
		switch op.Op {
		case "add":
			fallthrough
		case "remove":
			continue
		default:
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Only membership add/remove is allowed"}
		}
	}

	var pushErrors string

	// members are guaranteed to be multivalued
	for _, op := range operations {
		for _, singleVal := range casting.MultiValue[map[string]interface{}](op.Value) {
			value := casting.SingleValue[string](singleVal["value"])
			switch op.Op {
			case scim.PatchOperationAdd:
				if err := h.client.AddUserToGroup(value, id); err != nil {
					pushErrors += err.Error() + "\n"
					slog.Warn("Error adding user from group", "user", value, "error", err)
				}
			case scim.PatchOperationRemove:
				if err := h.client.RemoveUserFromGroup(value, id); err != nil {
					pushErrors += err.Error() + "\n"
					slog.Warn("Error removing user from group", "user", value, "error", err)
				}
			default:
				slog.Info("unknown OP", "operation", op.Op, "path", op.Path, "value", singleVal)
				continue
			}
		}
	}

	if len(pushErrors) > 0 {
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
