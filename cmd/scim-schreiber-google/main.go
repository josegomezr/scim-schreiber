package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/licensing/v1"
	"google.golang.org/api/option"

	"github.com/josegomezr/scim-schreiber-ldap/internal/logging"
	"github.com/josegomezr/scim-schreiber-ldap/internal/model"
)

type Config struct {
	Domain      string
	Credentials string
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

	credentials, ok := os.LookupEnv("GOOGLE_CREDENTIALS")

	if !ok {
		credentials = "service-account.json"
	}

	cfg := Config{
		Domain:      os.Getenv("GOOGLE_DOMAIN"),
		Credentials: credentials,
	}

	adminClient, err := createAdminClientDefault(&cfg)
	licenseClient, err := createLicenseClientDefault(&cfg)

	if err != nil {
		slog.Error("Failed to create client", "err", err)
		return
	}

	server, err := createSCIMServer(cfg, adminClient, licenseClient)

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

	accessLogger := logging.NewAccessLogger(server)
	mux.Handle("/", accessLogger)

	listenAddr := ":9440"
	slog.Info("Listening", "listenAddr", listenAddr)
	// TODO(josegomezr): configurable ports here
	err = http.ListenAndServe(listenAddr, mux)

	if err != nil {
		slog.Error("Failed to start http server", "err", err)
		return
	}
}

func createTokenSource(ctx context.Context, cfg *Config) (option.ClientOption, error) {
	b, err := os.ReadFile(cfg.Credentials)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
		return nil, err
	}

	scopes := []string{
		admin.AdminDirectoryUserScope,
		admin.AdminDirectoryGroupScope,
		admin.AdminDirectoryOrgunitScope,
		licensing.AppsLicensingScope,
	}

	config, err := google.JWTConfigFromJSON(b, scopes...)

	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
		return nil, err
	}

	subject, ok := os.LookupEnv("GOOGLE_SUBJECT")

	if ok {
		config.Subject = subject
	}

	ts := config.TokenSource(ctx)
	return option.WithTokenSource(ts), nil
}

func createAdminClientDefault(cfg *Config) (*admin.Service, error) {
	ctx := context.Background()
	source, err := createTokenSource(ctx, cfg)

	if err != nil {
		return nil, err
	}

	return createAdminClient(ctx, source)
}

func createAdminClient(ctx context.Context, opts ...option.ClientOption) (*admin.Service, error) {
	srv, err := admin.NewService(ctx, opts...)
	if err != nil {
		log.Fatalf("Unable to retrieve directory Client %v", err)
		return nil, err
	}
	return srv, nil
}

func createLicenseClientDefault(cfg *Config) (*licensing.Service, error) {
	ctx := context.Background()
	source, err := createTokenSource(ctx, cfg)

	if err != nil {
		return nil, err
	}

	return createLicenseClient(ctx, source)
}

func createLicenseClient(ctx context.Context, opts ...option.ClientOption) (*licensing.Service, error) {
	srv, err := licensing.NewService(ctx, opts...)
	if err != nil {
		log.Fatalf("Unable to retrieve licensing Client %v", err)
	}

	return srv, nil
}

func createSCIMServer(cfg Config, adminClient *admin.Service, licenseClient *licensing.Service) (scim.Server, error) {
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
		SupportPatch:     true,
		DocumentationURI: optional.NewString("http://nobody.cares/"),
	}

	resourceTypes := []scim.ResourceType{
		{
			ID:          optional.NewString("User"),
			Name:        "User",
			Endpoint:    "/Users",
			Description: optional.NewString("User Account"),
			Schema:      UserSchema,
			Handler: UserHandler{
				cfg:           &cfg,
				adminClient:   adminClient,
				licenseClient: licenseClient,
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
				client: adminClient,
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
