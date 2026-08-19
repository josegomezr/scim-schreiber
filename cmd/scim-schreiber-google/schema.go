package main

import (
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
)

const SCHEMA_GOOGLE_GROUP = "urn:ietf:params:scim:schemas:extension:google:2.0:CloudIdentityGroup"

var SchemaExtensionGoogleCloudIdentityGroup = schema.Schema{
	ID:          SCHEMA_GOOGLE_GROUP,
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

const SCHEMA_GOOGLE_USER = "urn:ietf:params:scim:schemas:extension:suse:2.0:GoogleUser"

// this is a custom extension, as Google does not publish one containing all their user attributes
var SchemaExtensionSUSEGoogleUser = schema.Schema{
	ID:          SCHEMA_GOOGLE_USER,
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
		schema.SimpleCoreAttribute(schema.SimpleBooleanParams(schema.BooleanParams{
			Name:       "isSupervisor",
			Required:   false,
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "workLocationType",
			Required:   false,
			Uniqueness: schema.AttributeUniquenessNone(),
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "office",
			Required:   false,
			Uniqueness: schema.AttributeUniquenessNone(),
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "l3leader",
			Required:   false,
			Uniqueness: schema.AttributeUniquenessNone(),
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "jobFamily",
			Required:   false,
			Uniqueness: schema.AttributeUniquenessNone(),
			Mutability: schema.AttributeMutabilityReadWrite(),
		})),
	},
}
