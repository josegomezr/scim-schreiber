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
			Supported: true,
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

func CastSingleValue[T any](input interface{}) T {
	output, _ := input.(T)
	return output
}

func CastMultiValue[T any](input interface{}) []T {
	var out []T
	for _, i := range input.([]interface{}) {
		out = append(out, CastSingleValue[T](i))
	}
	return out
}
