package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/elimity-com/scim"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/licensing/v1"
	"google.golang.org/api/option"
)

const (
	testUserUUID = "104965396198408263080"
)

type SCIMUserTestSuite struct {
	suite.Suite
	ctx    context.Context
	server scim.Server
}

func createApiClientWithoutCredentials() (*admin.Service, error) {
	ctx := context.Background()
	return createAdminClient(ctx, option.WithHTTPClient(http.DefaultClient))
}

func createLicenseClientWithoutCredentials() (*licensing.Service, error) {
	ctx := context.Background()
	return createLicenseClient(ctx, option.WithHTTPClient(http.DefaultClient))
}

func (suite *SCIMUserTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	cfg := Config{
		Domain: "dev.suse.com",
	}

	client, err := createApiClientWithoutCredentials()
	require.NoError(suite.T(), err)

	license, err := createLicenseClientWithoutCredentials()
	require.NoError(suite.T(), err)

	server, err := createSCIMServer(cfg, client, license)
	require.NoError(suite.T(), err)

	suite.server = server
}

func (suite *SCIMUserTestSuite) TestCreateUser() {
	t := suite.T()

	defer gock.Off()

	requestBody, err := os.Open(filepath.Join(".", "testdata", "create-user.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, requestBody.Close())
	})

	mockToken(t)
	mock(t, "https://admin.googleapis.com/admin/directory/v1/users", 201, "create-user-response.json")
	gock.New("https://licensing.googleapis.com").Get("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(404)
	gock.New("https://licensing.googleapis.com").Post("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPost, "/Users", requestBody)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)
}

func (suite *SCIMUserTestSuite) TestReplaceUser() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "replace-user.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	defer gock.Off()
	mockToken(t)
	mock(t, "https://admin.googleapis.com/admin/directory/v1/users/testuser@dev.suse.com", 200, "replace-user-response.json")
	mockOk(t, "https://licensing.googleapis.com/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com", "get_license.json")

	gock.New("https://licensing.googleapis.com").Delete("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPut, "/Users/testuser@dev.suse.com", file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()
	want := `
			{
			  "active" : true,
			  "displayName" : "Display Test",
			  "emails" : [ {
				"primary" : true,
				"type" : "work",
				"value" : "testuser@dev.suse.com"
			  }],
			  "externalId" : "testuser@dev.suse.com",
			  "id" : "100994131692123746646",
			  "meta" : {
				"resourceType" : "User",
				"location" : "Users/100994131692123746646"
			  },
			  "name" : {
				"familyName" : "User",
				"formatted" : "Test User",
				"givenName" : "Replace"
			  },
			  "entitlements": [],
			  "orgUnitPath" : "/Authentik Staging",
			  "schemas" : [ "urn:ietf:params:model:schemas:core:2.0:User" ],
			  "userName" : "testuser@dev.suse.com"
			}
	    `

	assert.JSONEq(t, want, got)
}

func (suite *SCIMUserTestSuite) TestPatchUser() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "user_change.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	defer gock.Off()
	mockToken(t)
	mock(t, "https://admin.googleapis.com/admin/directory/v1/users/"+testUserUUID, 200, "replace-user-response.json")
	mockOk(t, "https://licensing.googleapis.com/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com", "get_license.json")

	gock.New("https://licensing.googleapis.com").Delete("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPatch, "/Users/"+testUserUUID, file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()
	want := `
			{
			  "active" : true,
			  "displayName" : "Display Test",
			  "emails" : [ {
				"primary" : true,
				"type" : "work",
				"value" : "testuser@dev.suse.com"
			  }],
			  "externalId" : "testuser@dev.suse.com",
			  "id" : "100994131692123746646",
			  "meta" : {
				"resourceType" : "User",
				"location" : "Users/100994131692123746646"
			  },
			  "name" : {
				"familyName" : "User",
				"formatted" : "Test User",
				"givenName" : "Replace"
			  },
			  "entitlements": [
                {
					"value": "Google Workspace Enterprise Standard",
					"type": "license"
                }
              ],
			  "orgUnitPath" : "/Authentik Staging",
			  "schemas" : [ "urn:ietf:params:model:schemas:core:2.0:User" ],
			  "userName" : "testuser@dev.suse.com"
			}
	    `

	assert.JSONEq(t, want, got)
}

func (suite *SCIMUserTestSuite) TestDeleteUser() {
	t := suite.T()

	defer gock.Off()

	mockToken(t)
	gock.New("https://admin.googleapis.com/admin/directory/v1/users/" + testUserUUID).Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodDelete, "/Users/"+testUserUUID, nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func (suite *SCIMUserTestSuite) TestMissing() {
	t := suite.T()

	mockToken(t)
	mock(t, "https://admin.googleapis.com/admin/directory/v1/users/notAValidUUID", 404, "user_not_found.json")

	request, _ := http.NewRequest(http.MethodGet, "/Users/notAValidUUID", nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
	     {
	       "schemas": [ "urn:ietf:params:scim:api:messages:2.0:Error" ],
	       "status": "500",
           "detail": "googleapi: Error 404: Resource Not Found: userKey, notFound"
	    }
	    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
}

func (suite *SCIMUserTestSuite) TestGetUser() {
	t := suite.T()

	mockToken(t)
	mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users/104965396198408263080", "get_user.json")
	mockOk(t, "https://licensing.googleapis.com/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/felix-test@dev.suse.com", "get_license.json")

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Users/%s", testUserUUID), nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := fmt.Sprintf(`
     {
       "schemas": [ "urn:ietf:params:model:schemas:core:2.0:User" ],
       "externalId":"felix-test@dev.suse.com",
       "id":"%[1]s",
       "active": true,
       "userName":"felix-test@dev.suse.com",
       "displayName": "",
       "name": {
          "familyName":"Test", 
          "formatted":"Felix Test", 
          "givenName":"Felix"
       },
       "emails": [
            { "type": "work", "primary": true, "value": "felix-test@dev.suse.com" }
       ],
       "orgUnitPath":"/Authentik Staging",
	  "entitlements": [
		{"product":"Google-Apps", "sku":"1010020026"}
	  ],
       "meta": {
          "location": "Users/%[1]s",
          "resourceType":"User"
       }
    }
    `, testUserUUID)

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func mockOk(t *testing.T, url string, responseFile string) {
	mock(t, url, 200, responseFile)
}

func mock(t *testing.T, url string, status int, responseFile string) {
	file, err := os.Open(filepath.Join(".", "testdata", responseFile))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	gock.New(url).Reply(status).Body(file)
}

func mockToken(t *testing.T) {
	mockOk(t, "https://oauth2.googleapis.com/token", "token.json")
}

func (suite *SCIMUserTestSuite) TestGetAllUsers() {
	t := suite.T()

	defer gock.Off()
	mockToken(t)
	mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users", "all_users.json")

	request, _ := http.NewRequest(http.MethodGet, "/Users", nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
  "Resources" : [ 
   {
     "schemas": [ "urn:ietf:params:model:schemas:core:2.0:User" ],
     "externalId":"svc_authentik@dev.suse.com",
       "id":"113765969969838891919",
       "active": true,
       "userName":"svc_authentik@dev.suse.com",
       "displayName": "",
       "name": {
          "familyName":"authentik", 
          "formatted":"svc authentik", 
          "givenName":"svc"
       },
       "emails": [
            { "type": "work", "primary": true, "value": "svc_authentik@dev.suse.com" }
       ],
       "orgUnitPath":"/",
       "meta": {
          "location": "Users/113765969969838891919",
          "resourceType":"User"
       }
   },
   {
    "schemas": [ "urn:ietf:params:model:schemas:core:2.0:User" ],
     "externalId":"testokta@dev.suse.com",
       "id":"104963533172021742203",
       "active": false,
       "userName":"testokta@dev.suse.com",
       "displayName": "",
       "name": {
          "familyName":"Okta", 
          "formatted":"Test Okta", 
          "givenName":"Test"
       },
       "emails": [
            { "type": "work", "primary": true, "value": "testokta@dev.suse.com" }
       ],
       "orgUnitPath":"/CW Limited Access",
       "meta": {
          "location": "Users/104963533172021742203",
          "resourceType":"User"
       }
   }
  ],
  "itemsPerPage" : 100,
  "schemas" : [ "urn:ietf:params:scim:api:messages:2.0:ListResponse" ],
  "startIndex" : 1,
  "totalResults" : 2
}
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMUserTestSuite) TestFilterUsers() {
	t := suite.T()

	defer gock.Off()
	mockToken(t)
	mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users?alt=json&domain=dev.suse.com&prettyPrint=false&query=email%3Dfelix-test%40dev.suse.com", "filter_response.json")

	request, _ := http.NewRequest(http.MethodGet, "/Users?filter="+url.QueryEscape("userName eq \"felix-test@dev.suse.com\""), nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
  "Resources" : [{
       "schemas": [ "urn:ietf:params:model:schemas:core:2.0:User" ],
       "externalId":"felix-test@dev.suse.com",
       "id":"104965396198408263080",
       "active": true,
       "userName":"felix-test@dev.suse.com",
       "displayName": "",
       "name": {
          "familyName":"Test", 
          "formatted":"Felix Test", 
          "givenName":"Felix"
       },
       "emails": [
            { "type": "work", "primary": true, "value": "felix-test@dev.suse.com" }
       ],
       "orgUnitPath":"/Authentik Staging",
       "meta": {
          "location": "Users/104965396198408263080",
          "resourceType":"User"
       }
  }],
  "itemsPerPage" : 100,
  "schemas" : [ "urn:ietf:params:scim:api:messages:2.0:ListResponse" ],
  "startIndex" : 1,
  "totalResults" : 1
}
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestSCIMUserTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMUserTestSuite))
}
