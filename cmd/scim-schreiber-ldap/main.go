package main

import (
	"log"
	"log/slog"
	"net/http"
)

type Config struct {
	AllowUserCreation     bool
	GroupCreationIsUpsert bool
}

func main() {
	cfg := Config{
		AllowUserCreation:     false,
		GroupCreationIsUpsert: true,
	}

	conn, err := setupLdap()
	if err != nil {
		log.Fatalf("BORKED, LDAP NOT WORKING: %s", err)
	}
	conn.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /-/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	l := LDAPHandler{
		cfg: &cfg,
	}

	mux.HandleFunc("GET /v2/ServiceProviderConfig", l.ServiceProviderConfigHandler)
	mux.HandleFunc("GET /v2/Users", l.ListUsersHandler)
	mux.HandleFunc("GET /v2/Users/{user_uuid}", l.GetUserHandler)
	mux.HandleFunc("POST /v2/Users", l.CreateUserHandler)
	mux.HandleFunc("PUT /v2/Users/{user_id}", l.UpdateUserHandler)
	mux.HandleFunc("DELETE /v2/Users/{user_id}", l.DeleteUser)
	mux.HandleFunc("POST /v2/Groups", l.CreateGroupHandler)
	mux.HandleFunc("GET /v2/Groups/{group_id}", l.GetGroupHandler)
	mux.HandleFunc("PATCH /v2/Groups/{group_id}", l.UpdateGroupHandler)
	mux.HandleFunc("DELETE /v2/Groups/{user_id}", l.DeleteGroup)

	listenAddr := ":9440"
	slog.Info("Listening", "listenAddr", listenAddr)
	// TODO(josegomezr): configurable ports here
	http.ListenAndServe(listenAddr, l.ConnectMiddleware(mux))
}
