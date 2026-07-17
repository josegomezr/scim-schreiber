package main

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/elimity-com/scim"

	"github.com/josegomezr/scim-schreiber-ldap/internal/jira-server"
	"github.com/josegomezr/scim-schreiber-ldap/internal/server"
)

type Config struct {
	Token                      string
	ServerUrl                  string
	IncludeMembersInGroups     bool
	IgnoreGroupAddResponseCode bool
	GroupExclusionFile         string
}

func main() {
	slog.SetDefault(server.GetLogger())

	cfg := Config{
		Token:                      os.Getenv("JIRA_SERVER_TOKEN"),
		ServerUrl:                  os.Getenv("JIRA_SERVER_URL"),
		IncludeMembersInGroups:     os.Getenv("JIRA_INCLUDE_MEMBERS_IN_GROUPS") == "1",
		IgnoreGroupAddResponseCode: os.Getenv("JIRA_IGNORE_GROUP_ADD_RESPONSE_CODE") == "1",
		GroupExclusionFile:         os.Getenv("JIRA_GROUP_EXCLUSION_FILE"),
	}

	scimServer, err := createSCIMServer(cfg)
	if err != nil {
		slog.Error("Failed to create server", "err", err)
		return
	}

	server.StartHttpServer(scimServer)
}

func createSCIMServer(cfg Config) (scim.Server, error) {
	jiraClient, err := jira.NewClient(func(mscfg *jira.Config) {
		mscfg.Token = cfg.Token
		mscfg.IncludeMembersInGroups = cfg.IncludeMembersInGroups
		mscfg.ServerUrl = cfg.ServerUrl
	})

	if err != nil {
		return scim.Server{}, err
	}

	groupExclusions := GroupExclusions{}
	if cfg.GroupExclusionFile != "" {
		// TODO(josegomezr): make this less ugly, move it to a fn and return the struct pointer from there.
		f, err := os.Open(cfg.GroupExclusionFile)
		if err == nil {
			err = json.NewDecoder(f).Decode(&groupExclusions)
			if err == nil {
				slog.Debug("Loaded exclusions from config file", "file", cfg.GroupExclusionFile, "exclusions", groupExclusions)
				slog.Info("Loaded exclusions from config file", "file", cfg.GroupExclusionFile)
			}
		}
	}

	return server.NewSCIMServer(
		UserHandler{
			cfg:    &cfg,
			client: jiraClient,
		},
		GroupHandler{
			cfg:             &cfg,
			client:          jiraClient,
			groupExclusions: groupExclusions,
		},
	)
}
