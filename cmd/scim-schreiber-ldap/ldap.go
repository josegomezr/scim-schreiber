package main

import (
	"fmt"
	"iter"
	"log"
	"log/slog"
	"os"
	"strconv"

	"github.com/go-ldap/ldap/v3"
)

type LdapUtil struct {
	conn        *ldap.Conn
	endpoint    string
	bindDn      string
	bindPw      string
	baseUserOu  string
	baseGroupOu string
	baseDn      string
	dialOpts    []ldap.DialOpt
}

func LdapUtilFromEnv() LdapUtil {
	return LdapUtil{
		conn:        nil,
		endpoint:    os.Getenv("LDAP_URL"),
		bindDn:      os.Getenv("LDAP_BIND_DN"),
		bindPw:      os.Getenv("LDAP_BIND_PW"),
		baseUserOu:  os.Getenv("LDAP_BASE_USER_OU"),
		baseDn:      os.Getenv("LDAP_BASE_DN"),
		baseGroupOu: os.Getenv("LDAP_BASE_GROUP_OU"),
	}
}

func (l *LdapUtil) connect() error {
	conn, err := ldap.DialURL(l.endpoint, l.dialOpts...)
	if err != nil {
		return err
	}
	if err := conn.Bind(l.bindDn, l.bindPw); err != nil {
		return err
	}

	l.conn = conn
	return nil
}

func (l *LdapUtil) CreateGroup(id string, name string, gid int) (string, error) {
	dn := fmt.Sprintf("cn=%s,%s,%s", id, l.baseGroupOu, l.baseDn)
	addReq := ldap.NewAddRequest(dn, []ldap.Control{})
	addReq.Attribute("objectClass", []string{"top", "organization", "posixGroup"})
	addReq.Attribute("o", []string{name})
	addReq.Attribute("gidNumber", []string{strconv.Itoa(gid)})

	if err := l.conn.Add(addReq); err != nil {
		log.Fatal("error adding group:", addReq, err)
		return "", err
	}

	return dn, nil
}

func (l *LdapUtil) DeleteGroup(id string) error {
	dn := fmt.Sprintf("cn=%s,%s,%s", id, l.baseGroupOu, l.baseDn)
	err := l.conn.Del(ldap.NewDelRequest(dn, []ldap.Control{}))

	return err
}

func (l *LdapUtil) CreateUser(username string, password string, uuid string) (string, error) {
	dn := fmt.Sprintf("uid=%s,%s,%s", username, l.baseUserOu, l.baseDn)

	addReq := ldap.NewAddRequest(dn, []ldap.Control{})
	addReq.Attribute("objectClass", []string{"suseuser"})
	addReq.Attribute("sn", []string{"Surname"})
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

func (l *LdapUtil) DeleteUser(username string) error {
	dn := fmt.Sprintf("uid=%s,%s,%s", username, l.baseUserOu, l.baseDn)
	err := l.conn.Del(ldap.NewDelRequest(dn, []ldap.Control{}))

	return err
}

func (l *LdapUtil) disconnect() error {
	err := l.conn.Close()
	l.conn = nil
	return err
}

func (l *LdapUtil) GetGroup(id string) (*ldap.Entry, error) {

	filter := fmt.Sprintf("(cn=%s)", id)
	baseUid := l.baseGroupOu + "," + l.baseDn

	searchRequest := ldap.NewSearchRequest(
		baseUid,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1,
		0,
		false,
		filter, // The filter to apply
		nil,    // A list attributes to retrieve
		nil,
	)

	searchResult, err := l.conn.Search(searchRequest)
	if err != nil {
		slog.Error("Failed to execute search request", "err", err)
		return nil, err
	}

	if len(searchResult.Entries) != 1 {
		return nil, nil
	}

	return searchResult.Entries[0], nil
}

func (l *LdapUtil) SearchIter(filter string, baseUid string) iter.Seq2[*ldap.Entry, error] {
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
			response, err := l.conn.Search(searchRequest)

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

func (l *LdapUtil) UpdateEntry(dn string, adds map[string][]string, removes map[string][]string, replaces map[string][]string) error {
	request := ldap.NewModifyRequest(dn, nil)

	if adds != nil {
		for k, vals := range adds {
			request.Add(k, vals)
		}
	}
	if removes != nil {
		for k, vals := range removes {
			request.Delete(k, vals)
		}
	}
	if replaces != nil {
		for k, v := range replaces {
			request.Replace(k, v)
		}
	}
	fmt.Printf("adds: %+v\n", adds)
	fmt.Printf("removes: %+v\n", removes)
	fmt.Printf("replaces: %+v\n", replaces)
	return l.conn.Modify(request)
}

func (l *LdapUtil) searchGroups(uid string, field string) (iter.Seq2[*ldap.Entry, error], error) {
	filter := fmt.Sprintf("(%s=%s)", field, uid)
	baseUid := l.baseGroupOu + "," + l.baseDn
	return l.SearchIter(filter, baseUid), nil
}

func (l *LdapUtil) searchUsers(uid string, field string) (iter.Seq2[*ldap.Entry, error], error) {
	filter := fmt.Sprintf("(%s=%s)", field, uid)

	baseUid := l.baseUserOu + "," + l.baseDn
	return l.SearchIter(filter, baseUid), nil
}

func (l *LdapUtil) CountUsers() (int, error) {
	iterator, err := l.searchUsers("*", "uid")

	if err != nil {
		return 0, err
	}

	count := 0
	for range iterator {
		count++
	}

	return count, nil
}

func (l *LdapUtil) CountGroups() (int, error) {
	iterator, err := l.searchGroups("*", "cn")

	if err != nil {
		return 0, err
	}

	count := 0
	for range iterator {
		count++
	}

	return count, nil
}

func (l *LdapUtil) _searchUser(uid string, field string) *ldap.Entry {
	users, err := l.searchUsers(uid, field)
	if err != nil {
		return nil
	}

	for user, err := range users {
		if err != nil {
			return nil
		}
		return user
	}

	return nil
}

func (l *LdapUtil) searchUser(uid string) *ldap.Entry {
	return l._searchUser(uid, "uid")
}

func (l *LdapUtil) searchUserByUUID(uid string) *ldap.Entry {
	return l._searchUser(uid, "uuid")
}
