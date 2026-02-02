package main

import (
	"github.com/josegomezr/scim-schreiber-ldap/internal/scim"
)

func NewServiceProviderConfig() *scim.ServiceProviderConfig {
	return &scim.ServiceProviderConfig{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		AuthenticationSchemes: []scim.AuthScheme{
			{
				Type: "httpbasic",
				Name: "HTTP Basic",
			},
		},
		ChangePassword: scim.ServiceProviderSuportObj{
			Supported: false,
		},
		Patch: scim.ServiceProviderSuportObj{
			Supported: false,
		},
		Bulk: scim.ServiceProviderSuportObj{
			Supported: false,
		},
		Filter: scim.ServiceProviderSuportObj{
			Supported: true,
		},
		Sort: scim.ServiceProviderSuportObj{
			Supported: false,
		},
		Etag: scim.ServiceProviderSuportObj{
			Supported: false,
		},
	}
}
