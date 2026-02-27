package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elimity-com/scim"
	"github.com/josegomezr/scim-schreiber-ldap/cmd/scim-schreiber-ldap/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	if err != nil {
		log.Fatal(err)
	}
	suite.ldapContainer = ldapContainer

	endpoint, err := ldapContainer.GetEndpoint(suite.ctx)
	if err != nil {
		log.Fatal(err)
	}

	cfg := Config{
		AllowUserCreation:     false,
		GroupCreationIsUpsert: true,
	}

	server, err := createSCIMServer(cfg)

	if err != nil {
		log.Fatal(err)
	}
	suite.server = server

	ldap := LdapUtil{
		ldapEndpoint: endpoint,
		ldapBindDn:   "cn=Directory Manager",
		ldapBindPw:   "changeme",
		baseUserOu:   "ou=people",
		baseGroupOu:  "ou=groups",
		baseDn:       ldapContainer.BaseDN,
	}
	err = ldap.connect()
	if err != nil {
		log.Fatal(err)
	}

	suite.ldapCtx = ldap

}

func (suite *SCIMUserTestSuite) TearDownSuite() {
	if err := suite.ldapCtx.disconnect(); err != nil {
		log.Fatal(err)
	}

	if err := suite.ldapContainer.Terminate(suite.ctx); err != nil {
		log.Fatalf("error terminating postgres container: %s", err)
	}
}

func (suite *SCIMUserTestSuite) TestCreateCustomer() {
	t := suite.T()

	entry := suite.ldapCtx.searchUser("demo_user")
	require.NotNil(t, entry)
}

func (suite *SCIMUserTestSuite) TestLDAPMissing() {
	t := suite.T()

	request, _ := http.NewRequest(http.MethodGet, "/Users/demo_user", nil)
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
}

func (suite *SCIMUserTestSuite) TestGetUser() {
	t := suite.T()

	user := suite.ldapCtx.searchUser("demo_user")
	demoUserUuid := user.GetAttributeValue("uuid")

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/Users/%s", demoUserUuid), nil)
	ctx := WithLDAPContext(request.Context(), suite.ldapCtx)
	request = request.WithContext(ctx)

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

	//customer, err := suite.repository.GetCustomerByEmail(suite.ctx, "john@gmail.com")
	/*
		assert.NoError(t, err)
		assert.NotNil(t, customer)
		assert.Equal(t, "John", customer.Name)
		assert.Equal(t, "john@gmail.com", customer.Email)*/
}

func TestSCIMUserTestSuite(t *testing.T) {
	suite.Run(t, new(SCIMUserTestSuite))
}
