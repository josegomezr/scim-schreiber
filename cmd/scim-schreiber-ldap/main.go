package main

import (
	"log"
	"log/slog"

	"github.com/elimity-com/scim"

	"github.com/josegomezr/scim-schreiber-ldap/internal/server"
	"github.com/josegomezr/scim-schreiber-ldap/internal/uuidgenerator"
)

type Config struct {
	AllowUserCreation     bool
	GroupCreationIsUpsert bool
	UUIDGenerator         uuidgenerator.UUIDGenerator
}

func main() {
	slog.SetDefault(server.GetLogger())

	cfg := Config{
		AllowUserCreation:     false,
		GroupCreationIsUpsert: true,
		UUIDGenerator:         uuidgenerator.UUIDGeneratorImpl{},
	}

	testConnection()

	scimServer, err := createSCIMServer(cfg)
	if err != nil {
		slog.Error("Failed to create server", "err", err)
		return
	}

	server.StartHttpServer(scimServer, LdapMiddleware)
}

func testConnection() {
	l := LdapUtilFromEnv()

	err := l.connect()
	if err != nil {
		log.Fatalf("BORKED, LDAP NOT WORKING: %s", err)
	}
	l.disconnect()
}

func createSCIMServer(cfg Config) (scim.Server, error) {
	return server.NewSCIMServer(
		UserHandler{
			cfg:           &cfg,
			uuidGenerator: cfg.UUIDGenerator,
		},
		GroupHandler{
			cfg: &cfg,
		},
	)
}
