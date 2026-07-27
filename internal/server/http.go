package server

import (
	"github.com/elimity-com/scim"
	"log/slog"
	"net/http"

	"github.com/josegomezr/scim-schreiber-ldap/internal/logging"
)

type Middleware func(http.Handler) http.Handler

func StartHttpServer(server scim.Server, middlewares ...Middleware) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /-/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.Handle("/", logging.NewAccessLogger(server))

	var handler http.Handler = mux

	handler = FlattenPatchMiddleware(handler)

	for _, middleware := range middlewares {
		handler = middleware(handler)
	}

	listenAddr := ":9440"
	slog.Info("Listening", "listenAddr", listenAddr)
	// TODO(josegomezr): configurable ports here
	err := http.ListenAndServe(listenAddr, handler)

	if err != nil {
		slog.Error("Failed to start http server", "err", err)
		return
	}
}
