package main

import (
	ldap "github.com/go-ldap/ldap/v3"
	"iter"
	"log/slog"
	"os"
)

func connectAndBind(endpoint, bindDn, bindPw string) (*ldap.Conn, error) {
	l, err := ldap.DialURL(endpoint)
	if err != nil {
		return nil, err
	}
	if err := l.Bind(bindDn, bindPw); err != nil {
		return nil, err
	}
	return l, nil
}

func connectAndSearch(filter string) ([]*ldap.Entry, error) {
	endpoint := os.Getenv("LDAP_URL")
	bindDn := os.Getenv("LDAP_BIND_DN")
	bindPw := os.Getenv("LDAP_BIND_PW")

	conn, err := connectAndBind(endpoint, bindDn, bindPw)
	if err != nil {
		slog.Error("failed binding to LDAP", "endpoint", endpoint, "dn", bindDn, "error", err)
		return nil, err
	}
	defer conn.Close()
	return ldapSearch(conn, filter)
}

func ldapSearch(conn *ldap.Conn, filter string) ([]*ldap.Entry, error) {
	var baseUid string = os.Getenv("LDAP_BASE_DN")
	slog.Info("LDAP Search", "baseUid", baseUid, "filter", filter)
	// TODO: paginate
	searchRequest := ldap.NewSearchRequest(
		baseUid, // The base dn to search
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter, // The filter to apply
		nil,    // A list attributes to retrieve
		nil,
	)

	// TODO(josegomezr): make this an iterator, so it processes pages at a time instead of pulling all records into ram.
	sr, err := conn.SearchWithPaging(searchRequest, 1000)
	if err != nil {
		slog.Error("failed searching", "filter", filter, "error", err)
		return nil, err
	}
	return sr.Entries, nil
}

func ldapSearchIter(conn *ldap.Conn, filter string, baseUid string) iter.Seq2[*ldap.Entry, error] {
	slog.Info("LDAP Search (iterator)", "baseUid", baseUid, "filter", filter)

	pagingControl := ldap.NewControlPaging(1000)
	controls := []ldap.Control{pagingControl}

	return func(yield func(*ldap.Entry, error) bool) {
		for {
			slog.Info("Page")
			searchRequest := ldap.NewSearchRequest(
				baseUid,
				ldap.ScopeWholeSubtree,
				ldap.NeverDerefAliases,
				0,
				0,
				false,
				filter, // The filter to apply
				nil,    // A list attributes to retrieve
				controls,
			)
			response, err := conn.Search(searchRequest)

			if err != nil {
				slog.Error("Failed to execute search request", "err", err)
				yield(nil, err)
				return
			}

			for _, entry := range response.Entries {
				if !yield(entry, nil) {
					return
				}
			}

			// In order to prepare the next request, we check if the response
			// contains another ControlPaging object and a not-empty cookie and
			// copy that cookie into our pagingControl object:
			updatedControl := ldap.FindControl(response.Controls, ldap.ControlTypePaging)
			if ctrl, ok := updatedControl.(*ldap.ControlPaging); ctrl != nil && ok && len(ctrl.Cookie) != 0 {
				pagingControl.SetCookie(ctrl.Cookie)
				continue
			}
			// If no new paging information is available or the cookie is empty, we
			// are done with the pagination.
			break
		}
	}
}

func ldapFind(conn *ldap.Conn, dn string) (*ldap.Entry, error) {
	slog.Info("LDAP Find by DN", "dn", dn)
	// TODO: paginate
	searchRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(objectclass=*)",
		nil, // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		slog.Error("failed searching", "dn", dn, "error", err)
		return nil, err
	}
	if len(sr.Entries) == 0 {
		return nil, nil
	}
	return sr.Entries[0], nil
}

var LDAP_CONN *ldap.Conn
