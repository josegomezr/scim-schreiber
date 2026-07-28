package main

import (
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
)

var SchemaExtensionGoogleCloudIdentityGroup = schema.Schema{
	ID:          "urn:ietf:params:scim:schemas:extension:google:2.0:CloudIdentityGroup",
	Name:        optional.NewString("CloudIdentityGroup"),
	Description: optional.NewString("Google Cloud Identity Group attributes"),
	Attributes: []schema.CoreAttribute{
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "email",
			Required:   true,
			Uniqueness: schema.AttributeUniquenessServer(),
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "description",
			Required:   false,
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
	},
}

// this is a custom extension, as Google does not publish one containing all their user attributes
var SchemaExtensionSUSEGoogleUser = schema.Schema{
	ID:          "urn:ietf:params:scim:schemas:extension:suse:2.0:GoogleUser",
	Name:        optional.NewString("SUSE Google User Extension"),
	Description: optional.NewString("Custom Google Workspace User attributes"),
	Attributes: []schema.CoreAttribute{
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "orgUnitPath",
			Required:   false,
			Uniqueness: schema.AttributeUniquenessNone(),
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
		schema.ComplexCoreAttribute(schema.ComplexParams{
			Name:        "relations",
			Required:    false,
			MultiValued: true,
			Uniqueness:  schema.AttributeUniquenessNone(),
			Mutability:  schema.AttributeMutabilityReadWrite(),
			SubAttributes: []schema.SimpleParams{
				schema.SimpleStringParams(schema.StringParams{
					Name:     "type",
					Required: true,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "value",
					Required: true,
				}),
			},
		}),
	},
}
