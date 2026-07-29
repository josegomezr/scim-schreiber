package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/elimity-com/scim"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/licensing/v1"
	"google.golang.org/api/option"

	"github.com/josegomezr/scim-schreiber-ldap/internal/server"
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

	err = SyncSchemas(adminClient)
	if err != nil {
		slog.Error("Failed to sync schema", "err", err)
		return
	}

	scimServer, err := createSCIMServer(cfg, adminClient, licenseClient, NewProductInformationFromFile("products.yaml"))

	if err != nil {
		slog.Error("Failed to create server", "err", err)
		return
	}

	server.StartHttpServer(scimServer)
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
		admin.AdminDirectoryUserschemaScope,
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

func createSCIMServer(cfg Config, adminClient *admin.Service, licenseClient *licensing.Service, products *ProductInformation) (scim.Server, error) {
	config := server.NewSCIMConfig()
	config.SupportPatch = true

	return server.NewSCIMServer(
		UserHandler{
			cfg:                &cfg,
			adminClient:        adminClient,
			licenseClient:      licenseClient,
			productInformation: products,
		},
		GroupHandler{
			cfg:    &cfg,
			client: adminClient,
		},
		config,
		[]scim.SchemaExtension{
			{
				Schema:   SchemaExtensionSUSEGoogleUser,
				Required: true,
			},
		},
		[]scim.SchemaExtension{
			{
				Schema:   SchemaExtensionGoogleCloudIdentityGroup,
				Required: true,
			},
		},
	)
}
