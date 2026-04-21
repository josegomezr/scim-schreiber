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
	scim_filter_parser "github.com/scim2/filter-parser/v2"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"

	"github.com/josegomezr/scim-schreiber-ldap/internal/casting"
)

type GroupHandler struct {
	cfg    *Config
	client *admin.Service
}

func (h GroupHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
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

	return groupToGroupResource(group), nil
}

func (h GroupHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE /v2/Groups", "id", id)
	err := h.client.Groups.Delete(id).Do()
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

	group, err := h.client.Groups.Get(id).Do()
	if err != nil {

		var googleErr *googleapi.Error
		if errors.As(err, &googleErr) && googleErr.Code == http.StatusNotFound {
			return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
		}

		return scim.Resource{}, err
	}

	return groupToGroupResource(group), nil
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

func groupToGroupResource(entry *admin.Group) scim.Resource {
	return scim.Resource{
		ID:         entry.Id,
		ExternalID: optional.NewString(entry.Id),
		Attributes: map[string]interface{}{
			"displayName": entry.Name,
		},
	}
}

func resourceToGroup(resourceAttrs map[string]interface{}) *admin.Group {
	displayName := casting.SingleValue[string](resourceAttrs["displayName"])

	return &admin.Group{
		Name: displayName,
	}
}

func (h GroupHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Groups", "params", params)
	principal, err := displayNameFromFilter(params.FilterValidator)
	if err != nil {
		return scim.Page{}, err
	}

	// TODO Pagination
	request := h.client.Groups.List().Domain(h.cfg.Domain)

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
		resources = append(resources, groupToGroupResource(group))
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
				member := admin.Member{
					Email: value,
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

	if len(pushErrors) > 0 {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: pushErrors}
	}

	return scim.Resource{}, nil
}

func (h GroupHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Groups", "id", id, "attributes", attributes)

	groupRequest := resourceToGroup(attributes)
	group, err := h.client.Groups.Update(id, groupRequest).Do()
	if err != nil {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}
	return groupToGroupResource(group), nil
}
