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

type UserHandler struct {
	cfg    *Config
	client *jira.Client
}

func (h UserHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Users", "request", attributes)

	userRequest := resourceToJiraUser(attributes)
	user, err := h.client.CreateUser(*userRequest)
	if err != nil {
		if errors.Is(err, jira.UserAlreadyExists) {
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusConflict, Detail: err.Error()}
		}
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	return jiraUserToUserResource(user), nil
}

func (h UserHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE /v2/Users", "id", id)
	err := h.client.DeleteUser(id)
	if err != nil {
		return scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

func (h UserHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Users", "id", id)

	if id == "" {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound("")
	}

	jiraUser, err := h.client.GetUser(id)
	if err != nil {
		return scim.Resource{}, err
	}

	if jiraUser == nil {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
	}

	return jiraUserToUserResource(jiraUser), nil
}

func principalFromFilter(filterValidator *filter.Validator) (string, error) {
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
	if f.AttributePath.AttributeName != "userName" {
		return "", fmt.Errorf("only 'userName' is supported in filters")
	}
	return f.CompareValue.(string), nil
}

func jiraUserToUserResource(entry *jira.User) scim.Resource {
	return scim.Resource{
		ID:         entry.Key,
		ExternalID: optional.NewString(entry.Key),
		Attributes: map[string]interface{}{
			"rawUserLocation": entry.Self,
			"key":             entry.Key,
			"userName":        entry.Name,
			"displayName":     entry.DisplayName,
			"emailAddress":    entry.EmailAddress,
			"active":          entry.Active,
			"deleted":         entry.Deleted,
		},
	}
}

func resourceToJiraUser(resourceAttrs map[string]interface{}) *jira.User {
	emailList := casting.MultiValue[map[string]interface{}](resourceAttrs["emails"])
	email := ""
	if len(emailList) > 0 {
		for _, emailObj := range emailList {
			email = casting.SingleValue[string](emailObj["value"])
		}
	}
	return &jira.User{
		Name:         casting.SingleValue[string](resourceAttrs["userName"]),
		DisplayName:  casting.SingleValue[string](resourceAttrs["displayName"]),
		EmailAddress: email,
		Active:       casting.SingleValue[bool](resourceAttrs["active"]),
	}
}

func (h UserHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Users", "params", params)
	principal, err := principalFromFilter(params.FilterValidator)
	if err != nil {
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)

	i := 1
	for jiraUser, err := range h.client.ListAllUsers(principal) {
		if err != nil {
			return scim.Page{}, err
		}

		if i > (params.StartIndex + params.Count - 1) {
			// If we wanted to provide the correct result in TotalResults we'd actually have to keep counting here.
			break
		}

		if i >= params.StartIndex {
			resources = append(resources, jiraUserToUserResource(jiraUser))
		}
		i++
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func (h UserHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Users", "id", id, "operations", operations)
	return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Patch is not implemented for users"}
}

func (h UserHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Users", "id", id, "attributes", attributes)

	userRequest := resourceToJiraUser(attributes)
	jiraUser, err := h.client.UpdateUser(id, *userRequest)
	if err != nil {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}
	return jiraUserToUserResource(jiraUser), nil
}
