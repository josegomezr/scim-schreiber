package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elimity-com/scim"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/josegomezr/scim-schreiber-ldap/cmd/scim-schreiber-ldap/testhelpers"
)

import _ "embed"

const (
	testGroupId = "testGroupId"
)

//go:embed testdata/expectations/groups/replace-group-response.json
var replaceGroupResponse string

//go:embed testdata/expectations/groups/get-group.json
var getGroupResponse string

//go:embed testdata/expectations/groups/group-list.json
var groupListResponse string

//go:embed testdata/expectations/groups/filter-response.json
var filterResponse string

//go:embed testdata/stubs/groups/filter.json
var groupFilterStub string

type SCIMGroupTestSuite struct {
	suite.Suite
	ldapContainer *testhelpers.LdapContainer
	ctx           context.Context
	server        scim.Server
}

func (suite *SCIMGroupTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	cfg := Config{
		Domain: "dev.suse.com",
	}

	client, err := createApiClientWithoutCredentials()
	require.NoError(suite.T(), err)

	license, err := createLicenseClientWithoutCredentials()
	require.NoError(suite.T(), err)

	server, err := createSCIMServer(cfg, client, license, nil)
	require.NoError(suite.T(), err)

	suite.server = server
}

func (suite *SCIMGroupTestSuite) TestCreateGroup() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "requests", "groups", "create-group.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	defer gock.Off()
	gock.New("https://admin.googleapis.com").Post("/admin/directory/v1/groups").Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodPost, "/Groups", file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)
}

func (suite *SCIMGroupTestSuite) TestDeleteGroup() {
	t := suite.T()

	defer gock.Off()
	gock.New("https://admin.googleapis.com").Delete("/admin/directory/v1/groups/" + testGroupId).Reply(http.StatusNoContent)

	request, _ := http.NewRequest(http.MethodDelete, "/Groups/"+testGroupId, nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func (suite *SCIMGroupTestSuite) TestReplaceGroup() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "requests", "groups", "replace-group.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	gock.New("https://admin.googleapis.com").Put("/admin/directory/v1/groups/" + testGroupId).Reply(http.StatusOK).Body(strings.NewReader(`
		{"id": "1", "name":"Replaced Group", "email": "google-workspace-staging@dev.suse.com"}
   `))

	request, _ := http.NewRequest(http.MethodPut, "/Groups/"+testGroupId, file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()
	assert.JSONEq(t, replaceGroupResponse, got)
}

func (suite *SCIMGroupTestSuite) TestReplaceGroupWithPatch() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "requests", "groups", "patch_group.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	gock.New("https://admin.googleapis.com").Put("/admin/directory/v1/groups/" + testGroupId).Reply(http.StatusOK).Body(strings.NewReader(`
		{"id": "1", "name":"Replaced Group"}
   `))

	request, _ := http.NewRequest(http.MethodPatch, "/Groups/"+testGroupId, file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func (suite *SCIMGroupTestSuite) TestPatch() {
	t := suite.T()

	tests := map[string]struct {
		request  string
		status   int
		response string
	}{
		"addMemberPathStyle": {
			request: fmt.Sprintf(`
				{
				  "schemas": [
					"urn:ietf:params:scim:api:messages:2.0:PatchOp"
				  ],
				  "Operations": [
					 {
					 	"op": "add", "path": "members", "value": [{"value": "%s"}]
					 }
				  ]
				}
			`, testUserUUID),
			status: http.StatusNoContent,
		},
		"removeMemberPathStyle": {
			request: fmt.Sprintf(`
				{
				  "schemas": [
					"urn:ietf:params:scim:api:messages:2.0:PatchOp"
				  ],
				  "Operations": [
					 {
					 	"op": "remove", "path": "members", "value": [{"value": "%s"}]
					 }
				  ]
				}
			`, testUserUUID),
			status: http.StatusNoContent,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {

			defer gock.Off()
			gock.Observe(gock.DumpRequest)
			gock.New("https://admin.googleapis.com").Post("/admin/directory/v1/groups/group-" + name + "/members").Reply(http.StatusNoContent)
			gock.New("https://admin.googleapis.com").Delete("/admin/directory/v1/groups/group-" + name).Reply(http.StatusNoContent)

			request, _ := http.NewRequest(http.MethodPatch, "/Groups/group-"+name, strings.NewReader(test.request))

			response := httptest.NewRecorder()
			suite.server.ServeHTTP(response, request)

			assert.Equal(t, test.status, response.Code)
		})
	}
}

func (suite *SCIMGroupTestSuite) TestGetGroup() {
	t := suite.T()

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	gock.New("https://admin.googleapis.com").Get("/admin/directory/v1/groups/" + testGroupId).Reply(http.StatusOK).Body(strings.NewReader(`
		{"id": "1", "name":"Replaced Group", "email": "google-workspace-staging@dev.suse.com"}
   `))

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Groups/%s", testGroupId), nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()

	assert.JSONEq(t, getGroupResponse, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMGroupTestSuite) TestList() {
	t := suite.T()

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	suite.mockOk(t, "https://admin.googleapis.com/admin/directory/v1/groups", "list_groups.json")

	request, _ := http.NewRequest(http.MethodGet, "/Groups", nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := groupListResponse
	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMGroupTestSuite) TestFilter() {
	t := suite.T()

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	gock.New("https://admin.googleapis.com/admin/directory/v1/groups").MatchParam("query", "name='Test Group'").Reply(200).Body(strings.NewReader(groupFilterStub))

	request, _ := http.NewRequest(http.MethodGet, "/Groups?filter="+url.QueryEscape("displayName eq \"Test Group\""), nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()

	assert.JSONEq(t, filterResponse, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestSCIMGroupTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMGroupTestSuite))
}

func (suite *SCIMGroupTestSuite) mockOk(t *testing.T, url string, responseFile string) {
	suite.mock(t, url, 200, responseFile)
}

func (suite *SCIMGroupTestSuite) mock(t *testing.T, url string, status int, responseFile string) {
	file, err := os.Open(filepath.Join(".", "testdata", "stubs", "groups", responseFile))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	gock.New(url).Reply(status).Body(file)
}

func (suite *SCIMGroupTestSuite) mockToken(t *testing.T) {
	suite.mockOk(t, "https://oauth2.googleapis.com/token", "token.json")
}
