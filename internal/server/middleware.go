package server

import (
	"context"
	"net/http"
)

type contextKey string

const responseHeaderContextKey contextKey = "responseHeaderContextKey"

type ResponseHeaderData struct {
	Headers map[string]string
}

func SetResponseHeader(ctx context.Context, key string, value string) {
	raw := ctx.Value(responseHeaderContextKey)

	if raw == nil {
		return
	}

	w, ok := raw.(*http.ResponseWriter)

	if ok {
		(*w).Header().Set(key, value)
	}
}

func ResponseHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), responseHeaderContextKey, &w)

		// Call the next handler with the enriched context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
