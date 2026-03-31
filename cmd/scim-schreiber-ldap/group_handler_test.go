package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elimity-com/scim"
	"github.com/go-ldap/ldap/v3"
	"github.com/josegomezr/scim-schreiber-ldap/cmd/scim-schreiber-ldap/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testGroupId = "testGroupId"
)

type SCIMGroupTestSuite struct {
	suite.Suite
	ldapContainer *testhelpers.LdapContainer
	ctx           context.Context
	server        scim.Server
	ldapCtx       LdapUtil
}

func (suite *SCIMGroupTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	ldapContainer, err := testhelpers.CreateLdapContainer(suite.ctx)
	require.NoError(suite.T(), err)

	suite.ldapContainer = ldapContainer

	endpoint, err := ldapContainer.GetEndpoint(suite.ctx)
	require.NoError(suite.T(), err)

	cfg := Config{
		AllowUserCreation:     true,
		GroupCreationIsUpsert: true,
	}

	server, err := createSCIMServer(cfg)
	require.NoError(suite.T(), err)

	suite.server = server

	ldapUtil := LdapUtil{
		endpoint:    endpoint,
		bindDn:      "cn=Directory Manager",
		bindPw:      "changeme",
		baseUserOu:  "ou=people",
		baseGroupOu: "ou=groups",
		baseDn:      ldapContainer.BaseDN,
		dialOpts: []ldap.DialOpt{
			ldap.DialWithTLSConfig(&tls.Config{
				InsecureSkipVerify: true,
			}),
		},
	}
	err = ldapUtil.connect()
	if err != nil {
		log.Fatal(err)
	}

	suite.ldapCtx = ldapUtil

}

func (suite *SCIMGroupTestSuite) BeforeTest(suiteName, testName string) {
	_, err := suite.ldapCtx.CreateGroup(testGroupId, "Test Group", 1000)
	assert.NoError(suite.T(), err)

	_, err = suite.ldapCtx.CreateUser("test", "changeme", testUserUUID)
	assert.NoError(suite.T(), err)
}

func (suite *SCIMGroupTestSuite) AfterTest(suiteName, testName string) {
	err := suite.ldapCtx.DeleteUser("test")
	assert.NoError(suite.T(), err)

	err = suite.ldapCtx.DeleteGroup(testGroupId)
	assert.NoError(suite.T(), err)
}

func (suite *SCIMGroupTestSuite) TearDownSuite() {
	if err := suite.ldapCtx.disconnect(); err != nil {
		log.Fatal(err)
	}

	if err := suite.ldapContainer.Terminate(suite.ctx); err != nil {
		log.Fatalf("error terminating postgres container: %s", err)
	}
}

func (suite *SCIMGroupTestSuite) TestCreateGroup() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "create-group.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	request, _ := http.NewRequest(http.MethodPost, "/Groups", file)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
}

func (suite *SCIMGroupTestSuite) TestDeleteGroup() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodDelete, "/Groups/"+testGroupId, nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotImplemented, response.Code)
}

func (suite *SCIMGroupTestSuite) TestReplaceGroup() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "replace-group.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	request, _ := http.NewRequest(http.MethodPut, "/Groups/"+testGroupId, file)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotImplemented, response.Code)
}

func (suite *SCIMGroupTestSuite) TestPatch() {
	t := suite.T()

	tests := map[string]struct {
		request  string
		status   int
		response string
	}{
		"fullReplace": {
			request: `
				{
				  "schemas": [
					"urn:ietf:params:scim:api:messages:2.0:PatchOp"
				  ],
				  "Operations": [
					 {
					   "op":"replace",
					   "value":
						{
						  "displayName": "Patched Groupname"
						}
					 }
				  ]
				}
			`,
			status: http.StatusOK,
			response: `
				{
				  "displayName" : "Patched Groupname",
				  "externalId" : "cn=group-fullReplace,ou=groups,dc=suse,dc=com",
				  "id" : "group-fullReplace",
				  "members" : [ ],
				  "meta" : {
					"resourceType" : "Group",
					"location" : "Groups/group-fullReplace"
				  },
				  "schemas" : [ "urn:ietf:params:model:schemas:core:2.0:Group" ]
				}
			`,
		},
		"pathReplace": {
			request: `
				{
				  "schemas": [
					"urn:ietf:params:scim:api:messages:2.0:PatchOp"
				  ],
				  "Operations": [
					 {
					   "op":"replace",
					   "path":"displayName",
					   "value":"Patched Path Group"
					 }
				  ]
				}
			`,
			status: http.StatusOK,
			response: `
				{
				  "displayName" : "Patched Path Group",
				  "externalId" : "cn=group-pathReplace,ou=groups,dc=suse,dc=com",
				  "id" : "group-pathReplace",
				  "members" : [ ],
				  "meta" : {
					"resourceType" : "Group",
					"location" : "Groups/group-pathReplace"
				  },
				  "schemas" : [ "urn:ietf:params:model:schemas:core:2.0:Group" ]
				}
			`,
		},
		"addMember": {
			request: fmt.Sprintf(`
		  		{
		  		  "schemas": [
		  			"urn:ietf:params:scim:api:messages:2.0:PatchOp"
		  		  ],
		  		  "Operations": [
		  			 {
		  			   "op":"add",
		  			   "value": {
		  			   "members": [
		  				   {
		  						"display": "alex",
		  						"value": "%s"
		  				   }
		  			     ]
		  			   }
		  			 }
		  		  ]
		  		}
		  	`, testUserUUID),
			status: http.StatusOK,
			response: `
		  		{
		  		  "displayName" : "PATCH-addMember",
		  		  "externalId" : "cn=group-addMember,ou=groups,dc=suse,dc=com",
		  		  "id" : "group-addMember",
				  "members": [
					{
					  "value": "test"
					}
				  ],
		  		  "meta" : {
		  			"resourceType" : "Group",
		  			"location" : "Groups/group-addMember"
		  		  },
		  		  "schemas" : [ "urn:ietf:params:model:schemas:core:2.0:Group" ]
		  		}
		  	`,
		},
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
			status: http.StatusOK,
			// TODO In the request we expect the uuid for the member and in the response we give back the name?
			response: `
				{
				  "displayName": "PATCH-addMemberPathStyle",
				  "externalId": "cn=group-addMemberPathStyle,ou=groups,dc=suse,dc=com",
				  "id": "group-addMemberPathStyle",
				  "members": [
					{
					  "value": "test"
					}
				  ],
				  "meta": {
					"resourceType": "Group",
					"location": "Groups/group-addMemberPathStyle"
				  },
				  "schemas": [
					"urn:ietf:params:model:schemas:core:2.0:Group"
				  ]
				}
			`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			group, err := suite.ldapCtx.CreateGroup("group-"+name, "PATCH-"+name, 1000)
			assert.NoError(t, err)

			t.Cleanup(func() {
				suite.ldapCtx.DeleteGroup(group)
			})

			request, _ := http.NewRequest(http.MethodPatch, "/Groups/group-"+name, strings.NewReader(test.request))
			ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
			request = request.WithContext(ctx)

			response := httptest.NewRecorder()
			suite.server.ServeHTTP(response, request)

			assert.Equal(t, test.status, response.Code)
			got := response.Body.String()

			assert.JSONEq(t, test.response, got)
		})
	}
}

func (suite *SCIMGroupTestSuite) TestLDAPMissing() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Groups/notAValidUUID", nil)
	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
       "schemas": [ "urn:ietf:params:scim:api:messages:2.0:Error" ],
       "status": "500"
    }
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
}

func (suite *SCIMGroupTestSuite) TestGetGroup() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Groups/%s", testGroupId), nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
		{
		  "displayName": "Test Group",
		  "externalId": "cn=testGroupId,ou=groups,dc=suse,dc=com",
		  "id": "testGroupId",
		  "members": [],
		  "meta": {
			"resourceType": "Group",
			"location": "Groups/testGroupId"
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

	request, _ := http.NewRequest(http.MethodGet, "/Groups", nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
			{
			  "Resources": [
				{
				  "displayName": "",
				  "externalId": "cn=demo_group,ou=Groups,dc=suse,dc=com",
				  "id": "demo_group",
				  "members": [],
				  "meta": {
					"resourceType": "Group",
					"location": "Groups/demo_group"
				  },
				  "schemas": [
					"urn:ietf:params:model:schemas:core:2.0:Group"
				  ]
				},
				{
				  "displayName": "Test Group",
				  "externalId": "cn=testGroupId,ou=groups,dc=suse,dc=com",
				  "id": "testGroupId",
				  "members": [],
				  "meta": {
					"resourceType": "Group",
					"location": "Groups/testGroupId"
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
			  "totalResults": 3
			}
    `
	// TODO totalResults is borked
	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMGroupTestSuite) TestGetCount() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Groups?count=0", nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
       "Resources":null,
       "itemsPerPage":0,
       "schemas": ["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
       "startIndex":1,
       "totalResults":2
    }
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMGroupTestSuite) TestFilter() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Groups?filter="+url.QueryEscape("displayName eq \"Test Group\""), nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
		{
		  "Resources": [
			{
			  "displayName": "Test Group",
			  "externalId": "cn=testGroupId,ou=groups,dc=suse,dc=com",
			  "id": "testGroupId",
			  "members": [],
			  "meta": {
				"resourceType": "Group",
				"location": "Groups/testGroupId"
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

func TestSCIMGroupTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMGroupTestSuite))
}
