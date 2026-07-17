package server

import (
	"fmt"
	"log/slog"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
)

const (
	DUMMY_URI = "https://undefined.itpe.suse.com"
)

type ScimLogger struct {
}

func (c *ScimLogger) Error(args ...any) {
	slog.Error(fmt.Sprintln(args...))
}

func NewSCIMServer(userHandler scim.ResourceHandler, groupHandler scim.ResourceHandler) (scim.Server, error) {
	dummyUri := optional.NewString(DUMMY_URI)

	config := scim.ServiceProviderConfig{
		AuthenticationSchemes: []scim.AuthenticationScheme{
			{
				Type:             scim.AuthenticationTypeHTTPBasic,
				Name:             "HTTP Basic",
				DocumentationURI: dummyUri,
				SpecURI:          dummyUri,
			},
		},
		MaxResults:       100,
		SupportFiltering: true,
		DocumentationURI: dummyUri,
	}

	resourceTypes := []scim.ResourceType{
		{
			ID:          optional.NewString("User"),
			Name:        "User",
			Endpoint:    "/Users",
			Description: optional.NewString("User Account"),
			Schema:      schema.CoreUserSchema(),
			SchemaExtensions: []scim.SchemaExtension{
				{
					Schema:   schema.ExtensionEnterpriseUser(),
					Required: false,
				},
				{
					Schema:   SchemaExtensionSUSEUser,
					Required: false,
				},
			},
			Handler: userHandler,
		},
		{
			ID:          optional.NewString("Group"),
			Name:        "Group",
			Endpoint:    "/Groups",
			Description: optional.NewString("Groups"),
			Schema:      schema.CoreGroupSchema(),
			Handler:     groupHandler,
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
