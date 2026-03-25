package main

import (
	"fmt"
	"log/slog"
	// "io"
	// "os"
	// "encoding/json"
	"net/http"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/filter"
	"github.com/elimity-com/scim/optional"
	"github.com/josegomezr/scim-schreiber-ldap/internal/casting"
	"github.com/josegomezr/scim-schreiber-ldap/internal/msgraph"
	scim_filter_parser "github.com/scim2/filter-parser/v2"
)

type GroupHandler struct {
	cfg    *Config
	client *msgraph.Client
}

func (h GroupHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Groups", "request", attributes)

	groupRequest := resourceToMsGroup(attributes)
	group, err := h.client.CreateGroup(*groupRequest)
	if err != nil {
		return scim.Resource{}, errors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	return msGroupToGroupResource(group), nil
}

func (h GroupHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE /v2/Groups", "id", id)
	err := h.client.DeleteGroup(id)
	if err != nil {
		return errors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

func (h GroupHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Groups", "id", id)

	if id == "" {
		return scim.Resource{}, errors.ScimErrorResourceNotFound("")
	}

	msGroup, err := h.client.GetGroup(id)
	if err != nil {
		return scim.Resource{}, err
	}

	if msGroup == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	return msGroupToGroupResource(msGroup), nil
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
	if f.AttributePath.AttributeName != "groupName" {
		return "", fmt.Errorf("only 'groupName' is supported in filters")
	}
	return f.CompareValue.(string), nil
}

func msGroupToGroupResource(entry *msgraph.Group) scim.Resource {
	return scim.Resource{
		ID:         entry.Id,
		ExternalID: optional.NewString(entry.Id),
		Attributes: map[string]interface{}{
			"displayName":     entry.DisplayName,
			"createdDateTime": entry.CreatedDateTime,
		},
	}
}

func resourceToMsGroup(resourceAttrs map[string]interface{}) *msgraph.Group {
	return &msgraph.Group{
		DisplayName: casting.SingleValue[string](resourceAttrs["displayName"]),
	}
}

func (h GroupHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Groups", "params", params)
	principal, err := displayNameFromFilter(params.FilterValidator)
	if err != nil {
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)
	for msGroup, err := range h.client.ListAllGroups(principal) {
		if err != nil {
			return scim.Page{}, err
		}
		resources = append(resources, msGroupToGroupResource(msGroup))
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func (h GroupHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Groups", "id", id, "operations", operations)
	return scim.Resource{}, errors.ScimError{Status: http.StatusNotImplemented, Detail: "Patch is not implemented for groups"}
}

func (h GroupHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Groups", "id", id, "attributes", attributes)

	groupRequest := resourceToMsGroup(attributes)
	msGroup, err := h.client.UpdateGroup(id, *groupRequest)
	if err != nil {
		return scim.Resource{}, errors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}
	return msGroupToGroupResource(msGroup), nil
}
