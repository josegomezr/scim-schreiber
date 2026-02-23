package main

import (
	"net/http"
)

func (l *LdapUtil) ConnectMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := setupLdap()
		if err != nil {
			w.WriteHeader(500)
			return
		}
		defer conn.Close()
		l.conn = conn
		next.ServeHTTP(w, r)
	})
}
