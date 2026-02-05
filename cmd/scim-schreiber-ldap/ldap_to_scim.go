package main

import (
	ldap "github.com/go-ldap/ldap/v3"
	"github.com/josegomezr/scim-schreiber-ldap/internal/scim"
)

func ldapEntryToUserResponse(entry *ldap.Entry) *scim.UserRespOK {
	return &scim.UserRespOK{
		ExternalId: entry.DN,
		Id:         entry.GetAttributeValue("uuid"),
	}
}

func ldapEntryToGroupResponse(entry *ldap.Entry) *scim.GroupRespOK {
	members := []scim.ValueObj{}
	for _, value := range entry.GetAttributeValues("memberUid") {
		members = append(members, scim.ValueObj{
			Value: value,
		})
	}

	// TODO: Id's need something more qualifies, maybe we can use the UUID
	// generated in the IDP
	//
	// If not we should fallback to DN's everywhere.
	return &scim.GroupRespOK{
		Id:          entry.GetAttributeValue("cn"),
		ExternalId:  entry.DN,
		DisplayName: entry.GetAttributeValue("cn"),
		Members:     members,
	}
}
