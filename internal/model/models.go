package model

import (
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
)

type UserName struct {
	FamilyName string `json:"familyName"`
	Formatted  string `json:"formatted"`
	GivenName  string `json:"givenName"`
}

// TODO(josegomezr): Expand on the property mappings side and here what do we
// want to persist from authentik into LDAP

// https://datatracker.ietf.org/doc/html/rfc7643
var UserSchema = schema.Schema{
	ID:          "urn:ietf:params:model:schemas:core:2.0:User",
	Name:        optional.NewString("User"),
	Description: optional.NewString("User Account"),
	Attributes: []schema.CoreAttribute{
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:        "schemas",
			Required:    true,
			MultiValued: true,
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:        "sshPublicKey",
			Required:    false,
			MultiValued: true,
		})),
		schema.SimpleCoreAttribute(schema.SimpleBooleanParams(schema.BooleanParams{
			Name:     "active",
			Required: true,
		})),
		schema.ComplexCoreAttribute(schema.ComplexParams{
			Name:        "emails",
			Required:    true,
			MultiValued: true,
			SubAttributes: []schema.SimpleParams{
				schema.SimpleBooleanParams(schema.BooleanParams{
					Name:     "primary",
					Required: true,
				}),
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
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:     "externalId",
			Required: true,
		})),
		schema.ComplexCoreAttribute(schema.ComplexParams{
			Name:        "name",
			Required:    true,
			MultiValued: false,
			SubAttributes: []schema.SimpleParams{
				schema.SimpleStringParams(schema.StringParams{
					Name:     "familyName",
					Required: true,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "formatted",
					Required: true,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "givenName",
					Required: true,
				}),
			},
		}),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "userName",
			Required:   true,
			Uniqueness: schema.AttributeUniquenessServer(),
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:     "title",
			Required: false,
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:     "organization",
			Required: false,
		})),
		schema.ComplexCoreAttribute(schema.ComplexParams{
			Name:        "addresses",
			Required:    false,
			MultiValued: true,
			SubAttributes: []schema.SimpleParams{
				schema.SimpleStringParams(schema.StringParams{
					Name:     "streetAddress",
					Required: false,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "locality",
					Required: false,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "postalCode",
					Required: false,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "region",
					Required: false,
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:     "country",
					Required: false,
				}),
			},
		}),
		schema.ComplexCoreAttribute(schema.ComplexParams{
			Name:        "phoneNumbers",
			Required:    false,
			MultiValued: true,
			Description: optional.NewString(`
			Phone numbers for the User.  The value
			SHOULD be canonicalized by the service provider according to the
			format specified in RFC 3966, e.g., 'tel:+1-201-555-0123'.
			Canonical type values of 'work', 'home', 'mobile', 'fax', 'pager',
			and 'other'.
			`),
			SubAttributes: []schema.SimpleParams{
				schema.SimpleStringParams(schema.StringParams{
					Name:        "value",
					Required:    false,
					Description: optional.NewString("Phone number of the User."),
				}),
				schema.SimpleStringParams(schema.StringParams{
					Name:        "type",
					Required:    false,
					Description: optional.NewString("A label indicating the attribute's function, e.g., 'work', 'home', 'mobile'."),
				}),
			},
		}),
	},
}

// TODO(josegomezr): Expand on the property mappings side and here what do we
// want to persist from authentik into LDAP
var GroupSchema = schema.Schema{
	ID:          "urn:ietf:params:model:schemas:core:2.0:Group",
	Name:        optional.NewString("Group"),
	Description: optional.NewString("Group"),
	Attributes: []schema.CoreAttribute{
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:        "schemas",
			Required:    true,
			MultiValued: true,
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:       "externalId",
			Required:   true,
			Mutability: schema.AttributeMutabilityReadOnly(), // Write is currently not implemented for this
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:     "displayName",
			Required: true,
		})),
		schema.ComplexCoreAttribute(schema.ComplexParams{
			Name:        "members",
			Required:    false,
			MultiValued: true,
			SubAttributes: []schema.SimpleParams{
				schema.SimpleStringParams(schema.StringParams{
					Name:     "value",
					Required: true,
				}),
			},
		}),
		/*
			schema.ComplexCoreAttribute(schema.ComplexParams{
				Name:        "ldapDistinguishedName",
				Required:    true,
				MultiValued: false,
				SubAttributes: []schema.SimpleParams{
					schema.SimpleStringParams(schema.StringParams{
						Name:     "familyName",
						Required: true,
					}),
					schema.SimpleStringParams(schema.StringParams{
						Name:     "formatted",
						Required: true,
					}),
					schema.SimpleStringParams(schema.StringParams{
						Name:     "givenName",
						Required: true,
					}),
				},
			}),*/
	},
}

type ValueObj struct {
	Value string `json:"value"`
}
