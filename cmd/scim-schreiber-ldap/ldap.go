package main

import (
	"fmt"
	ldap "github.com/go-ldap/ldap/v3"
	"os"
)

// TODO(josegomezr): separate connect into a different function
func connectAndSearch(filter string) ([]*ldap.Entry, error) {
	// TODO(josegomezr): accept multiple bases
	// TODO(josegomezr): split user & group search projections
	var baseUid string = os.Getenv("LDAP_BASE_DN")

	endpoint := os.Getenv("LDAP_URL")
	l, err := ldap.DialURL(endpoint)
	if err != nil {
		return nil, err
	}
	defer l.Close()
	bindDn := os.Getenv("LDAP_BIND_DN")
	err = l.Bind(bindDn, os.Getenv("LDAP_BIND_PW"))
	if err != nil {
		fmt.Println("failed binding to LDAP", "endpoint", endpoint, "dn", bindDn, "error", err)
		return nil, err
	}

	filter = "(" + ldap.EscapeFilter(filter) + ")"
	fmt.Println("Search: base=", baseUid, "filter=", filter)
	searchRequest := ldap.NewSearchRequest(
		baseUid, // The base dn to search
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases, 0, 0, false,
		filter,                              // The filter to apply
		[]string{"dn", "uid", "uuid", "cn"}, // A list attributes to retrieve
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}
	return sr.Entries, nil
}
