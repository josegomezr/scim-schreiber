package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"github.com/josegomezr/scim-schreiber-ldap/internal/jira-server"
	"github.com/josegomezr/scim-schreiber-ldap/internal/model"
)

type Config struct {
	Token     string
	ServerUrl string
}

func main() {
	programLevel := new(slog.LevelVar)

	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		if err := programLevel.UnmarshalText([]byte(logLevel)); err != nil {
			slog.Info("Falling back to LOG_LEVEL Info")
		}
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel}))
	slog.SetDefault(logger)

	cfg := Config{
		Token:     os.Getenv("JIRA_SERVER_TOKEN"),
		ServerUrl: os.Getenv("JIRA_SERVER_URL"),
	}

	server, err := createSCIMServer(cfg)

	if err != nil {
		slog.Error("Failed to create server", "err", err)
		return
	}

	startHttpServer(server, err)
}

func startHttpServer(server scim.Server, err error) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /-/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.Handle("/", server)

	listenAddr := ":9440"
	slog.Info("Listening", "listenAddr", listenAddr)
	// TODO(josegomezr): configurable ports here
	err = http.ListenAndServe(listenAddr, mux)

	if err != nil {
		slog.Error("Failed to start http server", "err", err)
		return
	}
}

func createSCIMServer(cfg Config) (scim.Server, error) {
	jiraClient, err := jira.NewClient(func(mscfg *jira.Config) {
		mscfg.Token = cfg.Token
		mscfg.ServerUrl = cfg.ServerUrl
	})

	if err != nil {
		return scim.Server{}, err
	}

	config := scim.ServiceProviderConfig{
		AuthenticationSchemes: []scim.AuthenticationScheme{
			{
				Type:             scim.AuthenticationTypeHTTPBasic,
				Name:             "HTTP Basic",
				DocumentationURI: optional.NewString("http://nobody.cares/"),
				SpecURI:          optional.NewString("http://nobody.cares/"),
			},
		},
		MaxResults:       100,
		SupportFiltering: true,
		DocumentationURI: optional.NewString("http://nobody.cares/"),
	}

	resourceTypes := []scim.ResourceType{
		{
			ID:          optional.NewString("User"),
			Name:        "User",
			Endpoint:    "/Users",
			Description: optional.NewString("User Account"),
			Schema:      model.UserSchema,
			Handler: UserHandler{
				cfg:    &cfg,
				client: jiraClient,
			},
		},
		{
			ID:          optional.NewString("Group"),
			Name:        "Group",
			Endpoint:    "/Groups",
			Description: optional.NewString("Groups"),
			Schema:      model.GroupSchema,
			Handler: GroupHandler{
				cfg:    &cfg,
				client: jiraClient,
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

	return scim.NewServer(serverArgs, serverOpts...)
}
