package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/josegomezr/scim-schreiber-ldap/cmd/scim-schreiber-ldap/testhelpers"
	"github.com/josegomezr/scim-schreiber-ldap/internal/uuidgenerator"
)

const (
	testUserUUID = "2a19013f-6a7e-4293-8782-6275d43ca030"
)

type SCIMUserTestSuite struct {
	suite.Suite
	ldapContainer *testhelpers.LdapContainer
	ctx           context.Context
	server        http.Handler
	ldapCtx       LdapUtil
}

func (suite *SCIMUserTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	ldapContainer, err := testhelpers.CreateLdapContainer(suite.ctx)
	require.NoError(suite.T(), err)

	suite.ldapContainer = ldapContainer

	endpoint, err := ldapContainer.GetEndpoint(suite.ctx)
	require.NoError(suite.T(), err)

	cfg := Config{
		AllowUserCreation:     true,
		GroupCreationIsUpsert: true,
		UUIDGenerator:         uuidgenerator.UUIDGeneratorMock{},
	}

	s, err := createSCIMServer(cfg)
	require.NoError(suite.T(), err)

	suite.server = s

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

func (suite *SCIMUserTestSuite) BeforeTest(suiteName, testName string) {
	_, err := suite.ldapCtx.CreateTestUser("test", "changeme", testUserUUID)
	require.NoError(suite.T(), err)
}

func (suite *SCIMUserTestSuite) AfterTest(suiteName, testName string) {
	for _, user := range []string{
		"test", "jgomez", "noname",
	} {
		dn := fmt.Sprintf("uid=%s,%s,%s", user, suite.ldapCtx.baseUserOu, suite.ldapCtx.baseDn)
		err := suite.ldapCtx.Delete(dn)
		require.NoError(suite.T(), err)
	}
}

func (suite *SCIMUserTestSuite) TearDownSuite() {
	if err := suite.ldapCtx.disconnect(); err != nil {
		log.Fatal(err)
	}

	if err := suite.ldapContainer.Terminate(suite.ctx); err != nil {
		log.Fatalf("error terminating postgres container: %s", err)
	}
}

// TODO Use table tests
func (suite *SCIMUserTestSuite) TestCreateUserNoName() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "create-user-noname.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	request, _ := http.NewRequest(http.MethodPost, "/Users", file)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)
	got := response.Body.String()
	want := `
{
  "id": "noname",
  "name": {
    "familyName": "lastName",
    "formatted": "formatted"
  },
  "externalId": "uid=noname,ou=people,dc=suse,dc=com",
  "active": true,
  "emails": [
    {
      "primary": true,
      "type": "work",
      "value": "noname@suse.com"
    }
  ],
  "schemas": [
    "urn:ietf:params:scim:schemas:core:2.0:User",
    "urn:ietf:params:scim:schemas:extension:suse:2.0:User",
    "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
  ],
  "userName": "noname",
  "meta": {
    "location": "Users/noname",
    "resourceType": "User"
  }
}
    `

	assert.JSONEq(t, want, got)
}

func (suite *SCIMUserTestSuite) TestCreateUser() {
	t := suite.T()

	file, err := os.Open(filepath.Join(".", "testdata", "create-user.json"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	request, _ := http.NewRequest(http.MethodPost, "/Users", file)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusCreated, response.Code)
	got := response.Body.String()
	want := `
{
  "active": true,
    "emails": [
    {
      "primary": true,
      "type": "work",
      "value": "jose.gomez@suse.com"
    }
  ],
"externalId":"uid=jgomez,ou=people,dc=suse,dc=com",
  "userName": "jgomez",
  "id": "jgomez",
  "meta": {
    "location": "Users/jgomez",
    "resourceType": "User"
  },
  "name": {
    "givenName": "Jose",
    "familyName": "Gomez",
    "formatted":"José Gómez"
  },
  "urn:ietf:params:scim:schemas:extension:suse:2.0:User": {
    "communityUid": "josegomezr",
    "sshPublicKey": ["test-ssh-key"]
  },
  "schemas": [
    "urn:ietf:params:scim:schemas:core:2.0:User",
    "urn:ietf:params:scim:schemas:extension:suse:2.0:User",
    "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
  ]
}
    `

	assert.JSONEq(t, want, got)
}

func (suite *SCIMUserTestSuite) TestReplaceUser() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodPut, "/Users/test", strings.NewReader(replaceUserRequest))
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotImplemented, response.Code)
	got := response.Body.String()
	want := `
        {
          "status": "501",
          "detail":"replace is not implemented for users",
          "schemas": [
            "urn:ietf:params:scim:api:messages:2.0:Error"
          ]
        }
    `

	assert.JSONEq(t, want, got)
}

//go:embed testdata/replace-user.json
var replaceUserRequest string

func (suite *SCIMUserTestSuite) TestPatchUser() {
	t := suite.T()

	requestBody := `
        {
          "schemas": [
            "urn:ietf:params:scim:api:messages:2.0:PatchOp"
          ],
          "Operations": [
             {
               "op":"replace",
               "value": ` + replaceUserRequest + `
             }
          ]
        }
    `

	request, _ := http.NewRequest(http.MethodPatch, "/Users/test", strings.NewReader(requestBody))
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	got := response.Body.String()

	want := `
        {
          "name": { "givenName": "User", "formatted": "User Replace", "familyName": "Replace" },
          "externalId": "uid=test,ou=people,dc=suse,dc=com",
          "id": "test",
          "meta": {
            "resourceType": "User",
            "location": "Users/test"
          },
		  "active": false,	
          "emails": [
            { "type": "work", "primary": true, "value": "primary@suse.com" },
            { "type": "work", "primary": false, "value": "secondary@suse.com" }
          ],
          "title": "Test Engineer",
		  "addresses": [
			{
			  "streetAddress": "Streetname 1",
			  "locality": "City",
			  "region": "Bavaria",
			  "postalCode": "123456",
			  "country": "DE"
			}
		  ],
		  "phoneNumbers": [
			{
			  "type": "mobile",
			  "value": "+1 234"
			},
			{
			  "type": "work",
			  "value": "+1 456"
			}
		],
	"schemas" : [
	"urn:ietf:params:scim:schemas:core:2.0:User",
	"urn:ietf:params:scim:schemas:extension:suse:2.0:User",
	"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	],
	"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": {
		"organization": "SUSE"
	},
	"urn:ietf:params:scim:schemas:extension:suse:2.0:User": {
		"sshPublicKey": ["test-ssh-key"]
	},
          "userName": "test"
        }
    `

	assert.JSONEq(t, want, got)
}

func (suite *SCIMUserTestSuite) TestLDAPMissing() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users/notAValidUUID", nil)
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

func (suite *SCIMUserTestSuite) TestGetUser() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users/test", nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
	"schemas" : [
		"urn:ietf:params:scim:schemas:core:2.0:User",
		"urn:ietf:params:scim:schemas:extension:suse:2.0:User",
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	],
       "externalId":"uid=test,ou=people,dc=suse,dc=com",
       "id":"test",
  	   "active": true,
       "userName":"test",
       "name": {"familyName":"Surname", "formatted":"Max Mustermann", "givenName":"First Name"},
       "emails": [],
       "meta": {
          "location": "Users/test",
          "resourceType":"User"
       }
    }
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMUserTestSuite) TestGetAllUsers() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users", nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
  "Resources" : [ {
    "externalId" : "uid=test,ou=people,dc=suse,dc=com",
    "id" : "test",
    "userName":"test",
    "active": true,
	"name": {"familyName":"Surname", "formatted":"Max Mustermann", "givenName":"First Name"},
    "emails": [],
    "meta" : {
      "resourceType" : "User",
      "location" : "Users/test"
    },
	"schemas" : [
		"urn:ietf:params:scim:schemas:core:2.0:User",
		"urn:ietf:params:scim:schemas:extension:suse:2.0:User",
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	]
  } ],
  "itemsPerPage" : 100,
  "schemas" : [ "urn:ietf:params:scim:api:messages:2.0:ListResponse" ],
  "startIndex" : 1,
  "totalResults" : 2
}
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMUserTestSuite) TestGetUserCount() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users?count=0", nil)
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
       "totalResults":1
    }
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMUserTestSuite) TestFilterUsers() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users?filter="+url.QueryEscape("userName eq \"test\""), nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
     {
  "Resources" : [ {
	"name": {"familyName":"Surname", "formatted":"Max Mustermann", "givenName":"First Name"},
    "externalId" : "uid=test,ou=people,dc=suse,dc=com",
    "id" : "test",
    "active": true,
    "userName":"test",
    "emails": [],
    "meta" : {
      "resourceType" : "User",
      "location" : "Users/test"
    },
	"schemas" : [
		"urn:ietf:params:scim:schemas:core:2.0:User",
		"urn:ietf:params:scim:schemas:extension:suse:2.0:User",
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	]
  } ],
  "itemsPerPage" : 100,
  "schemas" : [ "urn:ietf:params:scim:api:messages:2.0:ListResponse" ],
  "startIndex" : 1,
  "totalResults" : 2
}
    `

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func (suite *SCIMUserTestSuite) TestFilterUsersWildcard() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users?filter="+url.QueryEscape("userName eq \"*\""), nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := `
    {
	  "Resources" : [],
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

func (l *LdapUtil) CreateTestUser(username string, password string, uuid string) (string, error) {
	dn := fmt.Sprintf("uid=%s,%s,%s", username, l.baseUserOu, l.baseDn)

	addReq := ldap.NewAddRequest(dn, []ldap.Control{})
	addReq.Attribute("objectClass", []string{"suseuser"})
	addReq.Attribute("sn", []string{"Surname"})
	addReq.Attribute("givenName", []string{"First Name"})
	addReq.Attribute("cn", []string{"Max Mustermann"})
	addReq.Attribute("uid", []string{username})
	addReq.Attribute("isActive", []string{"true"})
	addReq.Attribute("employeeNumber", []string{"1234"})
	addReq.Attribute("uuid", []string{uuid})

	if err := l.conn.Add(addReq); err != nil {
		log.Fatal("error adding user:", addReq, err)
		return "", err
	}

	modifyReq := ldap.NewPasswordModifyRequest(dn, "", password)
	_, err := l.conn.PasswordModify(modifyReq)
	if err != nil {
		log.Fatal("error setting user password:", err)
	}

	return dn, nil
}
