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

func NewSCIMConfig() *scim.ServiceProviderConfig {
	dummyUri := optional.NewString(DUMMY_URI)

	return &scim.ServiceProviderConfig{
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
}

func NewSCIMServer(userHandler scim.ResourceHandler, groupHandler scim.ResourceHandler, config *scim.ServiceProviderConfig, userExtensions []scim.SchemaExtension, groupExtensions []scim.SchemaExtension) (scim.Server, error) {
	if config == nil {
		config = NewSCIMConfig()
	}

	userExtensions = append(userExtensions, []scim.SchemaExtension{
		{
			Schema:   schema.ExtensionEnterpriseUser(),
			Required: false,
		},
	}...)

	groupExtensions = append(groupExtensions, []scim.SchemaExtension{}...)

	userSchema := schema.CoreUserSchema()
	userSchema.Attributes = append(userSchema.Attributes, schema.SchemasAttributes())
	userSchema.Attributes = append(userSchema.Attributes, schema.CommonAttributes()...)

	resourceTypes := []scim.ResourceType{
		{
			ID:               optional.NewString("User"),
			Name:             "User",
			Endpoint:         "/Users",
			Description:      optional.NewString("User Account"),
			Schema:           userSchema,
			SchemaExtensions: userExtensions,
			Handler:          userHandler,
		},
		{
			ID:               optional.NewString("Group"),
			Name:             "Group",
			Endpoint:         "/Groups",
			Description:      optional.NewString("Groups"),
			Schema:           schema.CoreGroupSchema(),
			SchemaExtensions: groupExtensions,
			Handler:          groupHandler,
		},
	}

	serverArgs := &scim.ServerArgs{
		ServiceProviderConfig: config,
		ResourceTypes:         resourceTypes,
	}

	serverOpts := []scim.ServerOption{
		scim.WithLogger(&ScimLogger{}), // optional, default is no logging
	}

	return scim.NewServer(serverArgs, serverOpts...)
}
