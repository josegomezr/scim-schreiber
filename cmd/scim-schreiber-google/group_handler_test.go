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

const (
	testGroupId = "testGroupId"
)

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

	server, err := createSCIMServer(cfg, client, license)
	require.NoError(suite.T(), err)

	suite.server = server
}

func (suite *SCIMGroupTestSuite) TestCreateGroup() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "create-group.json"))
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

	file, err := os.Open(filepath.Join(".", "testdata", "replace-group.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	gock.New("https://admin.googleapis.com").Put("/admin/directory/v1/groups/" + testGroupId).Reply(http.StatusOK).Body(strings.NewReader(`
		{"id": "1", "name":"Replaced Group"}
   `))

	request, _ := http.NewRequest(http.MethodPut, "/Groups/"+testGroupId, file)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()

	assert.JSONEq(t, `
		{
		  "displayName": "Replaced Group",
		  "externalId": "1",
		  "id": "1",
		  "meta": {
			"location": "Groups/1",
			"resourceType": "Group"
		  },
		  "schemas": [
			"urn:ietf:params:model:schemas:core:2.0:Group"
		  ]
		}
	`, got)
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
		{"id": "1", "name":"Replaced Group"}
   `))

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Groups/%s", testGroupId), nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
		{
		  "displayName": "Replaced Group",
		  "externalId": "1",
		  "id": "1",
		  "meta": {
			"location": "Groups/1",
			"resourceType": "Group"
		  },
		  "schemas": [
			"urn:ietf:params:model:schemas:core:2.0:Group"
		  ]
		}
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMGroupTestSuite) TestList() {
	t := suite.T()

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	mockOk(t, "https://admin.googleapis.com/admin/directory/v1/groups", "list_groups.json")

	request, _ := http.NewRequest(http.MethodGet, "/Groups", nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
			{
			  "Resources": [
				{
				  "displayName": "abuse",
				  "externalId": "04iylrwe1ryosyl",
				  "id": "04iylrwe1ryosyl",
				  "meta": {
					"location": "Groups/04iylrwe1ryosyl",
					"resourceType": "Group"
				  },
				  "schemas": [
					"urn:ietf:params:model:schemas:core:2.0:Group"
				  ]
				},
				{
				  "displayName": "postmaster",
				  "externalId": "00tyjcwt2wgxqro",
				  "id": "00tyjcwt2wgxqro",
				  "meta": {
					"location": "Groups/00tyjcwt2wgxqro",
					"resourceType": "Group"
				  },
				  "schemas": [
					"urn:ietf:params:model:schemas:core:2.0:Group"
				  ]
				}
			  ],
			  "itemsPerPage": 100,
			  "schemas": [
				"urn:ietf:params:scim:api:messages:2.0:ListResponse"
			  ],
			  "startIndex": 1,
			  "totalResults": 2
			}
    `
	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMGroupTestSuite) TestFilter() {
	t := suite.T()

	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	gock.New("https://admin.googleapis.com/admin/directory/v1/groups").MatchParam("query", "name='Test Group'").Reply(200).Body(strings.NewReader(`
			{
			  "kind": "admin#directory#groups",
			  "etag": "\"DcQ1N-bWFowTUUTzr_P6Kb_8lgiFBgpcxrHgSbp-VdA/DFKY_t69AnNE0fzrAxtib7CJVDg\"",
			  "groups": [
				{
				  "kind": "admin#directory#group",
				  "id": "04iylrwe1ryosyl",
				  "etag": "\"DcQ1N-bWFowTUUTzr_P6Kb_8lgiFBgpcxrHgSbp-VdA/2M_ysMr-t4PMM2aF3Byt-m7kVZU\"",
				  "email": "abuse@dev.suse.com",
				  "name": "Test Group",
				  "directMembersCount": "0",
				  "description": "",
				  "adminCreated": true,
				  "nonEditableAliases": [
					"abuse@dev.suse.com.test-google-a.com"
				  ]
				}
			  ]
			}
	`))

	request, _ := http.NewRequest(http.MethodGet, "/Groups?filter="+url.QueryEscape("displayName eq \"Test Group\""), nil)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
		{
		  "Resources": [
			{
			  "displayName": "Test Group",
			  "externalId": "04iylrwe1ryosyl",
			  "id": "04iylrwe1ryosyl",
			  "meta": {
				"resourceType": "Group",
				"location": "Groups/04iylrwe1ryosyl"
			  },
			  "schemas": [
				"urn:ietf:params:model:schemas:core:2.0:Group"
			  ]
			}
		  ],
		  "itemsPerPage": 100,
		  "schemas": [
			"urn:ietf:params:scim:api:messages:2.0:ListResponse"
		  ],
		  "startIndex": 1,
		  "totalResults": 1
		}
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestSCIMGroupTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMGroupTestSuite))
}
