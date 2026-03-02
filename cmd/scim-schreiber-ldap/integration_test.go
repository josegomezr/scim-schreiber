package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/elimity-com/scim"
	"github.com/go-ldap/ldap/v3"
	"github.com/josegomezr/scim-schreiber-ldap/cmd/scim-schreiber-ldap/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testUserUUID = "2a19013f-6a7e-4293-8782-6275d43ca030"
)

type SCIMUserTestSuite struct {
	suite.Suite
	ldapContainer *testhelpers.LdapContainer
	ctx           context.Context
	server        scim.Server
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
	}

	server, err := createSCIMServer(cfg)
	require.NoError(suite.T(), err)

	suite.server = server

	ldapUtil := LdapUtil{
		ldapEndpoint: endpoint,
		ldapBindDn:   "cn=Directory Manager",
		ldapBindPw:   "changeme",
		baseUserOu:   "ou=people",
		baseGroupOu:  "ou=groups",
		baseDn:       ldapContainer.BaseDN,
		dialOpts: []ldap.DialOpt{
			ldap.DialWithTLSConfig(&tls.Config{
				InsecureSkipVerify: true, // TODO Configure
			}),
		},
	}
	err = ldapUtil.connect()
	if err != nil {
		log.Fatal(err)
	}

	suite.ldapCtx = ldapUtil

	_, err = ldapUtil.CreateUser("test", "changeme", testUserUUID)
	if err != nil {
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

	assert.Equal(t, http.StatusNotImplemented, response.Code)
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

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Users/%s", testUserUUID), nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

	response := httptest.NewRecorder()
	suite.server.ServeHTTP(response, request)

	got := response.Body.String()
	want := fmt.Sprintf(`
 	{
       "schemas": [ "urn:ietf:params:model:schemas:core:2.0:User" ],
       "externalId":"CN=test,ou=people,dc=suse,dc=com",
       "id":"%[1]s",
       "meta": {
          "location": "Users/%[1]s",
          "resourceType":"User"
       }
	}
    `, testUserUUID)

	assert.JSONEq(t, want, got)
	assert.Equal(t, http.StatusOK, response.Code)
}

func TestSCIMUserTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMUserTestSuite))
}
