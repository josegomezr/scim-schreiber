package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

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
	server http.Handler
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

	s, err := createSCIMServer(cfg, client, license, NewProductInformationFromFile("testdata/products.yaml"))
	require.NoError(suite.T(), err)

	suite.server = s
}

func (suite *SCIMUserTestSuite) TestCreateUser() {
	t := suite.T()

	defer gock.Off()

	requestBody, err := os.Open(filepath.Join(".", "testdata", "requests", "users", "create-user.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, requestBody.Close())
	})

	suite.mockToken(t)
	suite.mock(t, "https://admin.googleapis.com/admin/directory/v1/users", 201, "create-user-response.json")
	gock.New("https://licensing.googleapis.com").Get("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(404)
	gock.New("https://licensing.googleapis.com").Post("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPost, "/Users", requestBody)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)
}

//go:embed testdata/expectations/users/replace-user.json
var replaceUserExpectation string

func (suite *SCIMUserTestSuite) TestReplaceUser() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "requests", "users", "replace-user.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		file.Close()
	})

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	suite.mockToken(t)
	suite.mockWithRequestBody(t, "https://admin.googleapis.com/admin/directory/v1/users/"+testUserUUID, 200, "replace-user-request.json", "replace-user-response.json")
	suite.mockOk(t, "https://licensing.googleapis.com/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com", "get_license.json")

	suite.mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users/"+testUserUUID+"/aliases", "alias.json")

	gock.New("https://licensing.googleapis.com").Delete("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPut, "/Users/"+testUserUUID, file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()
	want := replaceUserExpectation

	assert.JSONEq(t, want, got)
}

//go:embed testdata/expectations/users/patch-user.json
var patchUserExpectation string

func (suite *SCIMUserTestSuite) TestPatchUser() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "requests", "users", "user_change.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		file.Close()
	})

	defer gock.Off()
	suite.mockToken(t)
	suite.mock(t, "https://admin.googleapis.com/admin/directory/v1/users/"+testUserUUID, 200, "replace-user-response.json")
	suite.mockOk(t, "https://licensing.googleapis.com/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com", "get_license.json")

	gock.New("https://licensing.googleapis.com").Delete("/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/testuser@dev.suse.com").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPatch, "/Users/"+testUserUUID, file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()
	want := patchUserExpectation

	assert.JSONEq(t, want, got)
}

func (suite *SCIMUserTestSuite) TestDeleteUser() {
	t := suite.T()

	defer gock.Off()

	suite.mockToken(t)
	gock.New("https://admin.googleapis.com/admin/directory/v1/users/" + testUserUUID).Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodDelete, "/Users/"+testUserUUID, nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func (suite *SCIMUserTestSuite) TestMissing() {
	t := suite.T()

	suite.mockToken(t)
	suite.mock(t, "https://admin.googleapis.com/admin/directory/v1/users/notAValidUUID", 404, "user_not_found.json")

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

//go:embed testdata/expectations/users/get-user.json
var getUserExpectation string

func (suite *SCIMUserTestSuite) TestGetUser() {
	t := suite.T()

	suite.mockToken(t)
	suite.mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users/104965396198408263080", "get_user.json")
	suite.mockOk(t, "https://licensing.googleapis.com/apps/licensing/v1/product/Google-Apps/sku/1010020026/user/first.last@example.com", "get_license.json")

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Users/%s", testUserUUID), nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := getUserExpectation

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMUserTestSuite) mockOk(t *testing.T, url string, responseFile string) {
	suite.mock(t, url, 200, responseFile)
}

func (suite *SCIMUserTestSuite) mock(t *testing.T, url string, status int, responseFile string) {
	file, err := os.Open(filepath.Join(".", "testdata", "stubs", "users", responseFile))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	gock.New(url).Reply(status).Body(file)
}

func (suite *SCIMUserTestSuite) mockWithRequestBody(t *testing.T, url string, status int, requestFile string, responseFile string) {
	stub, err := os.Open(filepath.Join(".", "testdata", "stubs", "users", responseFile))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, stub.Close())
	})

	requestExpectation, err := os.Open(filepath.Join(".", "testdata", "expectations", "users", requestFile))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, requestExpectation.Close())
	})

	gock.New(url).Body(requestExpectation).Reply(status).Body(stub)
}

func (suite *SCIMUserTestSuite) mockToken(t *testing.T) {
	suite.mockOk(t, "https://oauth2.googleapis.com/token", "token.json")
}

//go:embed testdata/expectations/users/all-users.json
var allUsersExpectation string

func (suite *SCIMUserTestSuite) TestGetAllUsers() {
	t := suite.T()

	defer gock.Off()
	suite.mockToken(t)
	suite.mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users?projection=full", "all_users.json")

	request, _ := http.NewRequest(http.MethodGet, "/Users", nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := allUsersExpectation

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

//go:embed testdata/expectations/users/filter-users.json
var filterUserExpectation string

func (suite *SCIMUserTestSuite) TestFilterUsers() {
	t := suite.T()

	defer gock.Off()
	suite.mockToken(t)
	suite.mockOk(t, "https://admin.googleapis.com/admin/directory/v1/users?alt=json&domain=dev.suse.com&prettyPrint=false&query=email%3Dfelix-test%40dev.suse.com&projection=full", "filter_response.json")

	request, _ := http.NewRequest(http.MethodGet, "/Users?filter="+url.QueryEscape("userName eq \"felix-test@dev.suse.com\""), nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := filterUserExpectation

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestSCIMUserTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMUserTestSuite))
}
