package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/scim2/filter-parser/v2"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/licensing/v1"

	"github.com/josegomezr/scim-schreiber-ldap/internal/casting"
	"github.com/josegomezr/scim-schreiber-ldap/internal/model"
)

type UserHandler struct {
	cfg           *Config
	adminClient   *admin.Service
	licenseClient *licensing.Service
}

type Product struct {
	ProductId string
	SkuId     string
}

// https://developers.google.com/workspace/admin/licensing/v1/how-tos/products
var Products = map[string]Product{
	"Google Workspace Enterprise Standard": {
		ProductId: "Google-Apps",
		SkuId:     "1010020026",
	},
}

var ReverseProductMap = reverseProductMap(Products)

func reverseProductMap(productMap map[string]Product) map[string]map[string]string {
	m := make(map[string]map[string]string)

	for skuName, product := range productMap {
		productId := product.ProductId
		skuId := product.SkuId
		if m[productId] == nil {
			m[productId] = make(map[string]string)
		}
		m[productId][skuId] = skuName
	}

	return m
}

func (h UserHandler) Create(_ *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("POST /v2/Users", "request", attributes)

	userRequest, err := resourceToUser(attributes)

	if err != nil {
		return scim.Resource{}, err
	}

	// Google forces us to set a password...
	userRequest.Password = rand.Text()

	user, err := h.adminClient.Users.Insert(userRequest).Do()
	if err != nil {
		var googleErr *googleapi.Error
		if errors.As(err, &googleErr) && googleErr.Code == http.StatusConflict {
			return scim.Resource{}, scimerrors.ScimError{Status: http.StatusConflict, Detail: googleErr.Error()}
		}
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	wantLicenses, err := h.updateLicenses(user, attributes)
	if err != nil {
		return scim.Resource{}, err
	}
	resource := userToUserResource(user)
	resource.Attributes["entitlements"] = wantLicenses
	return resource, nil
}

func (h UserHandler) Delete(_ *http.Request, id string) error {
	slog.Info("DELETE /v2/Users", "id", id)
	err := h.adminClient.Users.Delete(id).Do()
	if err != nil {
		return scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	return nil
}

// Get the user.
// id: Identifies the user in the API request. The value can be the
// user's primary email address, alias email address, or unique user ID.
func (h UserHandler) Get(_ *http.Request, id string) (scim.Resource, error) {
	slog.Info("GET /v2/Users", "id", id)

	if id == "" {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound("")
	}

	user, err := h.adminClient.Users.Get(id).Do()
	if err != nil {
		slog.Warn("Error getting user", "id", id, "error", err)
		return scim.Resource{}, err
	}

	if user == nil {
		return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
	}

	resource := userToUserResource(user)

	licensesForUser, err := h.getLicenses(user)

	if err != nil {
		return scim.Resource{}, err
	}

	resource.Attributes["entitlements"] = licenseToResource(licensesForUser)

	return resource, nil
}

func (h UserHandler) getLicenses(user *admin.User) ([]Product, error) {
	licensesForUser := make([]Product, 0)

	for _, product := range Products {
		_, err := h.licenseClient.LicenseAssignments.Get(product.ProductId, product.SkuId, user.PrimaryEmail).Do()

		if err != nil {
			var googleErr *googleapi.Error
			if errors.As(err, &googleErr) && googleErr.Code == http.StatusNotFound {
				continue
			}

			return nil, err
		}

		licensesForUser = append(licensesForUser, product)
	}
	return licensesForUser, nil
}

func licenseToResource(licenses []Product) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(licenses))
	for _, assignment := range licenses {
		out = append(out, map[string]interface{}{
			"value": ReverseProductMap[assignment.ProductId][assignment.SkuId],
			"type":  "license",
		})
	}
	return out
}

func emailToResource(entry *admin.User) []map[string]interface{} {
	input, ok := entry.Emails.([]interface{})

	if !ok {
		return []map[string]interface{}{}
	}

	out := make([]map[string]interface{}, 0, len(input))
	for _, e := range input {
		mail := e.(map[string]interface{})
		// Skip internal mails (i.e. <username>.test-google-a.com)
		if slices.Contains(entry.NonEditableAliases, mail["address"].(string)) {
			continue
		}

		primary, ok := mail["primary"]
		if !ok {
			primary = false
		}

		out = append(out, map[string]interface{}{
			"primary": primary,
			"value":   mail["address"],
			"type":    "work",
		})
	}
	return out
}

func userToUserResource(entry *admin.User) scim.Resource {
	return scim.Resource{
		ID:         entry.Id,
		ExternalID: optional.NewString(entry.PrimaryEmail),
		Attributes: map[string]interface{}{
			"name": map[string]interface{}{
				"familyName": entry.Name.FamilyName,
				"givenName":  entry.Name.GivenName,
				"formatted":  entry.Name.FullName,
			},
			"emails":      emailToResource(entry),
			"displayName": entry.Name.DisplayName,
			"userName":    entry.PrimaryEmail,
			"active":      !entry.Suspended,
			"orgUnitPath": entry.OrgUnitPath,
		},
	}
}

func resourceToUser(resourceAttrs map[string]interface{}) (*admin.User, error) {
	nameMap := casting.SingleValue[map[string]interface{}](resourceAttrs["name"])

	emails, ok := resourceAttrs["emails"].([]interface{})

	primaryEmail := ""
	if ok {
		for _, e := range emails {
			email, ok := e.(map[string]interface{})
			if ok && email["primary"].(bool) {
				primaryEmail = email["value"].(string)
			}
		}
	}

	if primaryEmail == "" {
		slog.Warn("Primary email not found")
		return nil, scimerrors.ScimErrorBadRequest("Need a primary email")
	}

	userName := casting.SingleValue[string](resourceAttrs["userName"])

	if primaryEmail != userName {
		slog.Warn("Need a primary email to match username")
		return nil, scimerrors.ScimErrorBadRequest("Need a primary email to match username")
	}

	return &admin.User{
		PrimaryEmail: userName,
		Name: &admin.UserName{
			DisplayName: casting.SingleValue[string](resourceAttrs["displayName"]),
			FamilyName:  casting.SingleValue[string](nameMap["familyName"]),
			GivenName:   casting.SingleValue[string](nameMap["givenName"]),
			FullName:    casting.SingleValue[string](nameMap["formatted"]),
		},
		OrgUnitPath: casting.SingleValue[string](resourceAttrs["orgUnitPath"]),
		Suspended:   !casting.SingleValue[bool](resourceAttrs["active"]),
	}, nil
}

func (h UserHandler) GetAll(_ *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Info("GET /v2/Users", "params", params)
	principal, err := model.PrincipalFromFilter(params.FilterValidator)
	if err != nil {
		slog.Info("Invalid filter", "err", err)
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0)

	request := h.adminClient.Users.List().Domain(h.cfg.Domain)

	if principal != "" {
		request = request.Query("email=" + principal)
	}

	// Pagination is not implemented here. But we also don't really need it.
	users, err := request.Do()

	if err != nil {
		return scim.Page{}, err
	}

	for _, user := range users.Users {
		resources = append(resources, userToUserResource(user))
	}

	return scim.Page{
		TotalResults: len(resources),
		Resources:    resources,
	}, nil
}

func isEmptyPath(path *filter.Path) bool {
	return path == nil || path.String() == ""
}

func (h UserHandler) Patch(_ *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Info("PATCH /v2/Users", "id", id, "operations", operations)

	if len(operations) != 1 {
		slog.Warn("Only one replace is allowed")
		return scim.Resource{}, scimerrors.ScimErrorBadRequest("Only one replace is allowed")
	}

	operation := operations[0]

	if operation.Op != scim.PatchOperationReplace || !isEmptyPath(operation.Path) {
		slog.Warn("Only full replace is allowed")
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusNotImplemented, Detail: "Only full replace is allowed"}
	}

	attributes, ok := operation.Value.(map[string]interface{})

	if !ok {
		slog.Warn("Value must be a JSON object")
		return scim.Resource{}, scimerrors.ScimErrorBadRequest("Value must be a JSON object")
	}

	return h.updateUser(attributes, id)

}

func (h UserHandler) Replace(_ *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	slog.Info("PUT /v2/Users", "id", id, "attributes", attributes)

	return h.updateUser(attributes, id)
}

func (h UserHandler) updateUser(attributes scim.ResourceAttributes, id string) (scim.Resource, error) {
	userRequest, err := resourceToUser(attributes)

	if err != nil {
		return scim.Resource{}, err
	}

	user, err := h.adminClient.Users.Update(id, userRequest).Do()
	if err != nil {
		return scim.Resource{}, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	wantLicenses, err := h.updateLicenses(user, attributes)
	if err != nil {
		return scim.Resource{}, err
	}

	resource := userToUserResource(user)
	resource.Attributes["entitlements"] = wantLicenses
	return resource, nil
}

func (h UserHandler) updateLicenses(user *admin.User, attributes scim.ResourceAttributes) ([]map[string]interface{}, error) {
	hasLicenses, err := h.getLicenses(user)

	if err != nil {
		return nil, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
	}

	wantLicenses := []Product{}
	// Remove licenses is user is marked as inactive
	if !user.Suspended {
		tmp, ok := attributes["entitlements"]
		if ok {
			tmpCast, ok := tmp.([]interface{})
			if ok {
				for _, element := range tmpCast {
					if license, ok := element.(map[string]interface{}); ok {
						if license["type"] == "license" {
							wantLicenses = append(wantLicenses, Products[license["value"].(string)])
						}
					}
				}
			}
		}
	}

REMOVE:
	for _, l := range hasLicenses {
		for _, w := range wantLicenses {
			if l.ProductId == w.ProductId && l.SkuId == w.SkuId {
				continue REMOVE
			}
		}

		_, err := h.licenseClient.LicenseAssignments.Delete(l.ProductId, l.SkuId, user.PrimaryEmail).Do()
		if err != nil {
			var googleErr *googleapi.Error
			if errors.As(err, &googleErr) && googleErr.Code == http.StatusBadRequest {
				// This means that the license is auto-assigned and can't be removed manually.
				continue
			}
			return nil, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
		}

	}

ADD:
	for _, w := range wantLicenses {
		for _, l := range hasLicenses {
			if l.ProductId == w.ProductId && l.SkuId == w.SkuId {
				continue ADD
			}
		}

		license := licensing.LicenseAssignmentInsert{
			UserId: user.PrimaryEmail,
		}
		_, err := h.licenseClient.LicenseAssignments.Insert(w.ProductId, w.SkuId, &license).Do()
		if err != nil {
			return nil, scimerrors.ScimError{Status: http.StatusInternalServerError, Detail: fmt.Sprintf("%s", err)}
		}
	}
	return licenseToResource(wantLicenses), nil
}
