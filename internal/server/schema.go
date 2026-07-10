package server

import (
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"
)

var SchemaExtensionSUSEUser = schema.Schema{
	// TODO: this is not an IETF registered URN, either register it or find a different namespace
	ID:          "urn:ietf:params:scim:schemas:extension:suse:2.0:User",
	Name:        optional.NewString("SUSE User Extension"),
	Description: optional.NewString("Custom user attributes"),
	Attributes: []schema.CoreAttribute{
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:        "communityUid",
			MultiValued: false,
			Required:    false,
		})),
		schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
			Name:        "sshPublicKey",
			MultiValued: true,
			Required:    false,
		})),
	},
}
