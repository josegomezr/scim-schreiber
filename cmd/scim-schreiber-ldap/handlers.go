package main

import (
	"net/http"
)

func (l *LdapUtil) ConnectMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := l.connect()
		if err != nil {
			w.WriteHeader(500)
			return
		}
		defer l.disconnect()
		next.ServeHTTP(w, r)
	})
}
