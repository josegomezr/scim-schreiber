package main

import (
	"context"
	"net/http"
	"os"
)

type contextKey string

const ldapContextKey contextKey = "ldap-context"

func WithLDAPContext(ctx context.Context, l LdapUtil) context.Context {
	return context.WithValue(ctx, ldapContextKey, l)
}

func GetLDAPContext(ctx context.Context) (*LdapUtil, bool) {
	tc, ok := ctx.Value(ldapContextKey).(LdapUtil)
	return &tc, ok
}

func LdapMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		l := LdapUtil{
			conn:         nil,
			ldapEndpoint: os.Getenv("LDAP_URL"),
			ldapBindDn:   os.Getenv("LDAP_BIND_DN"),
			ldapBindPw:   os.Getenv("LDAP_BIND_PW"),
			baseUserOu:   os.Getenv("LDAP_BASE_USER_OU"),
			baseDn:       os.Getenv("LDAP_BASE_DN"),
			baseGroupOu:  os.Getenv("LDAP_BASE_GROUP_OU"),
		}

		err := l.connect()
		if err != nil {
			w.WriteHeader(500)
			return
		}
		defer l.disconnect()

		ctx := WithLDAPContext(r.Context(), l)

		// Call the next handler with the enriched context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
