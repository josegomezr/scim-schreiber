package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/filter"
	"github.com/elimity-com/scim/optional"
	"github.com/josegomezr/scim-schreiber-ldap/internal/utils"
	scim_filter_parser "github.com/scim2/filter-parser/v2"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"

	"github.com/josegomezr/scim-schreiber-ldap/internal/casting"
)

type GroupHandler struct {
	cfg    *Config
	client *admin.Service
}

func (h GroupHandler) Create(_ *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Groups", "request", attributes)

	groupRequest := resourceToGroup(attributes)
	group, err := h.client.Groups.Insert(groupRequest).Do()
	if err != nil {
		var googleErr *googleapi.Error
		if errors.As(err, &googleErr) && googleErr.Code == http.StatusConflict {
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusConflict, Detail: err.Error()}
		}
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return h.groupToGroupResource(group, false)
}

func (h GroupHandler) Delete(_ *http.Request, id string) error {
	slog.Info("DELETE /v2/Groups", "id", id)
	err := h.client.Groups.Delete(id).Do()
	if err != nil {
		return scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

func (h GroupHandler) Get(_ *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Groups", "id", id)

	if id == "" {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound("")
	}

	group, err := h.client.Groups.Get(id).Do()
	if err != nil {

		var googleErr *googleapi.Error
		if errors.As(err, &googleErr) && googleErr.Code == http.StatusNotFound {
			return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
		}

		slog.Warn("Error getting group", "id", id, "error", err)
		return scim.Resource{}, scimerrors.ScimErrorInternal
	}

	return h.groupToGroupResource(group, true)
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

func (h GroupHandler) groupToGroupResource(entry *admin.Group, fetchMembers bool) (scim.Resource, error) {

	googleExt := map[string]interface{}{
		"email": entry.Email,
	}

	attributes := map[string]interface{}{
		"displayName":       entry.Name,
		SCHEMA_GOOGLE_GROUP: googleExt,
	}

	if fetchMembers {
		membersAttribute := make([]map[string]interface{}, 0)

		// Check if this feature is enabled.
		// By returning an empty list we avoid authentik removing members it doesn't know about.
		if h.cfg.IncludeMembersInGroups {
			err := h.client.Members.List(entry.Id).Pages(context.Background(), func(members *admin.Members) error {
				for _, member := range members.Members {
					membersAttribute = append(membersAttribute, map[string]interface{}{
						"value": member.Id,
					})
				}
				return nil
			})

			if err != nil {
				slog.Warn("Error getting group members", "id", entry.Id, "error", err)
				return scim.Resource{}, scimerrors.ScimErrorInternal
			}
		}

		attributes["members"] = membersAttribute
	}

	return scim.Resource{
		ID:         entry.Id,
		ExternalID: optional.NewString(entry.Id),
		Attributes: attributes,
	}, nil
}

func resourceToGroup(resourceAttrs map[string]interface{}) *admin.Group {
	displayName := casting.SingleValue[string](resourceAttrs["displayName"])
	email := ""
	if val, ok := utils.GetExtensionAttribute(resourceAttrs, SCHEMA_GOOGLE_GROUP, "email"); ok {
		email = casting.SingleValue[string](val)
	}

	return &admin.Group{
		Name:  displayName,
		Email: email,
	}
}

func (h GroupHandler) GetAll(_ *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Groups", "params", params)
	principal, err := displayNameFromFilter(params.FilterValidator)
	if err != nil {
		return scim.Page{}, err
	}

	// TODO Pagination
	request := h.client.Groups.List().Domain(h.cfg.Domain).MaxResults(100)

	if principal != "" {
		filterExpr := fmt.Sprintf(`name='%s'`, principal)
		request = request.Query(filterExpr)
	}

	groups, err := request.Do()

	if err != nil {
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)
	for _, group := range groups.Groups {
		resource, err := h.groupToGroupResource(group, false)

		if err != nil {
			return scim.Page{}, err
		}

		resources = append(resources, resource)
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func (h GroupHandler) Patch(_ *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Groups", "id", id, "operations", operations)

	var pushErrors string

	for _, op := range operations {
		if op.Path == nil && op.Op == scim.PatchOperationReplace {
			attributes, ok := op.Value.(map[string]interface{})

			if !ok {
				slog.Warn("Value must be a JSON object")
				return scim.Resource{}, scimerrors.ScimErrorBadRequest("Value must be a JSON object")
			}

			groupRequest := resourceToGroup(attributes)
			_, err := h.client.Groups.Update(id, groupRequest).Do()
			if err != nil {
				pushErrors += err.Error() + "\n"
				slog.Warn("Error replacing group", "error", err)
			}
		} else if op.Path != nil && op.Path.String() == "members" {
			for _, singleVal := range casting.MultiValue[map[string]interface{}](op.Value) {
				value := casting.SingleValue[string](singleVal["value"])
				switch op.Op {
				case scim.PatchOperationAdd:
					member := admin.Member{
						Id: value,
					}
					if _, err := h.client.Members.Insert(id, &member).Do(); err != nil {
						pushErrors += err.Error() + "\n"
						slog.Warn("Error adding user from group", "user", value, "error", err)
					}
				case scim.PatchOperationRemove:
					if err := h.client.Members.Delete(id, value).Do(); err != nil {
						pushErrors += err.Error() + "\n"
						slog.Warn("Error removing user from group", "user", value, "error", err)
					}
				default:
					slog.Info("unknown OP", "operation", op.Op, "path", op.Path, "value", singleVal)
					continue
				}
			}
		}
	}

	if len(pushErrors) > 0 {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: pushErrors}
	}

	return scim.Resource{}, nil
}

func (h GroupHandler) Replace(_ *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Groups", "id", id, "attributes", attributes)

	groupRequest := resourceToGroup(attributes)
	group, err := h.client.Groups.Update(id, groupRequest).Do()
	if err != nil {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}
	return h.groupToGroupResource(group, true)
}
