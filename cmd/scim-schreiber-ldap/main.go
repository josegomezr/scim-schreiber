package main

import (
	"log"
	"log/slog"
	"net/http"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"github.com/josegomezr/scim-schreiber-ldap/internal/model"
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

	config := scim.ServiceProviderConfig{
		AuthenticationSchemes: []scim.AuthenticationScheme{
			{
				Type: scim.AuthenticationTypeHTTPBasic,
				Name: "HTTP Basic",
			},
		},
		SupportFiltering: true,
		SupportPatch:     true,
	}

	l := LdapUtil{}

	resourceTypes := []scim.ResourceType{
		{
			ID:          optional.NewString("User"),
			Name:        "User",
			Endpoint:    "/Users",
			Description: optional.NewString("User Account"),
			Schema:      model.UserSchema,
			Handler: UserHandler{
				ldap: &l,
				cfg:  &cfg,
			},
		},
		{
			ID:          optional.NewString("Group"),
			Name:        "Group",
			Endpoint:    "/Groups",
			Description: optional.NewString("Groups"),
			Schema:      model.GroupSchema,
			Handler: GroupHandler{
				ldap: &l,
				cfg:  &cfg,
			},
		},
	}

	serverArgs := &scim.ServerArgs{
		ServiceProviderConfig: &config,
		ResourceTypes:         resourceTypes,
	}

	serverOpts := []scim.ServerOption{
		scim.WithLogger(&ScimLogger{}), // optional, default is no logging
	}

	server, err := scim.NewServer(serverArgs, serverOpts...)

	if err != nil {
		slog.Error("Failed to create server", "err", err)
		return
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /-/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.Handle("/", server)

	listenAddr := ":9440"
	slog.Info("Listening", "listenAddr", listenAddr)
	// TODO(josegomezr): configurable ports here
	err = http.ListenAndServe(listenAddr, l.ConnectMiddleware(mux))

	if err != nil {
		slog.Error("Failed to start http server", "err", err)
		return
	}
}
