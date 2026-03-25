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

type UserHandler struct {
	cfg    *Config
	client *msgraph.Client
}

func (h UserHandler) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Users", "request", attributes)

	userRequest := resourceToMsUser(attributes)
	user, err := h.client.CreateUser(*userRequest)
	if err != nil {
		return scim.Resource{}, errors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	return msUserToUserResource(user), nil
}

func (h UserHandler) Delete(r *http.Request, id string) error {
	slog.Info("DELETE /v2/Users", "id", id)
	err := h.client.DeleteUser(id)
	if err != nil {
		return errors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

func (h UserHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Users", "id", id)

	if id == "" {
		return scim.Resource{}, errors.ScimErrorResourceNotFound("")
	}

	msUser, err := h.client.GetUser(id)
	if err != nil {
		return scim.Resource{}, err
	}

	if msUser == nil {
		return scim.Resource{}, errors.ScimErrorResourceNotFound(id)
	}

	return msUserToUserResource(msUser), nil
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

func msUserToUserResource(entry *msgraph.User) scim.Resource {
	return scim.Resource{
		ID:         entry.Id,
		ExternalID: optional.NewString(entry.OnPremisesImmutableId),
		Attributes: map[string]interface{}{
			"givenName":         entry.GivenName,
			"surname":           entry.Surname,
			"userName":          entry.MailNickname,
			"displayName":       entry.DisplayName,
			"mailNickname":      entry.MailNickname,
			"userPrincipalName": entry.UserPrincipalName,
			"accountEnabled":    entry.AccountEnabled,
		},
	}
}

func resourceToMsUser(resourceAttrs map[string]interface{}) *msgraph.User {
	nameMap := casting.SingleValue[map[string]interface{}](resourceAttrs["name"])

	return &msgraph.User{
		GivenName:             casting.SingleValue[string](nameMap["givenName"]),
		Surname:               casting.SingleValue[string](nameMap["familyName"]),
		DisplayName:           casting.SingleValue[string](resourceAttrs["displayName"]),
		MailNickname:          casting.SingleValue[string](resourceAttrs["userName"]),
		UserPrincipalName:     casting.SingleValue[string](resourceAttrs["userName"]),
		AccountEnabled:        casting.SingleValue[bool](resourceAttrs["active"]),
		OnPremisesImmutableId: casting.SingleValue[string](resourceAttrs["externalId"]),
	}
}

func (h UserHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Users", "params", params)
	principal, err := principalFromFilter(params.FilterValidator)
	if err != nil {
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)
	for msUser, err := range h.client.ListAllUsers(principal) {
		if err != nil {
			return scim.Page{}, err
		}
		resources = append(resources, msUserToUserResource(msUser))
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func (h UserHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Users", "id", id, "operations", operations)
	return scim.Resource{}, errors.ScimError{Status: http.StatusNotImplemented, Detail: "Patch is not implemented for users"}
}

func (h UserHandler) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Users", "id", id, "attributes", attributes)

	userRequest := resourceToMsUser(attributes)
	msUser, err := h.client.UpdateUser(id, *userRequest)
	if err != nil {
		return scim.Resource{}, errors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}
	return msUserToUserResource(msUser), nil
}
