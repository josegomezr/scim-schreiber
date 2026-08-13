//go:build !google_custom

package main

import (
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
)

func SyncSchemas(_ *admin.Service) error {
	// noop
	return nil
}

type CustomFields struct {
}

func UnmarshallCustomSchemas(customSchemas map[string]googleapi.RawMessage) CustomFields {
	return CustomFields{}
}

func CustomResourceToUser(resourceAttrs map[string]interface{}, googleAddresses []admin.UserAddress, aliases []string) (map[string]googleapi.RawMessage, error) {
	return map[string]googleapi.RawMessage{}, nil
}

func (c *CustomFields) GetUserType() string {
	return ""
}

func UpdateSCIMExtensions(customFields CustomFields, googleExt map[string]interface{}, enterpriseExt map[string]interface{}) {
	// noop
}
