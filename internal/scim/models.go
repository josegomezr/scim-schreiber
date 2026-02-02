package scim

type AuthScheme struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ServiceProviderSuportObj struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
	MaxResults     int  `json:"maxResults"`
}

type ServiceProviderConfig struct {
	Schemas               []string                 `json:"schemas"`
	AuthenticationSchemes []AuthScheme             `json:"authenticationSchemes"`
	Patch                 ServiceProviderSuportObj `json:"patch"`
	Bulk                  ServiceProviderSuportObj `json:"bulk"`
	Filter                ServiceProviderSuportObj `json:"filter"`
	ChangePassword        ServiceProviderSuportObj `json:"changePassword"`
	Sort                  ServiceProviderSuportObj `json:"sort"`
	Etag                  ServiceProviderSuportObj `json:"etag"`
}

type UserEmail struct {
	Primary bool   `json:"primary"`
	Type    string `json:"type"`
	Value   string `json:"value"`
}

type UserName struct {
	FamilyName string `json:"familyName"`
	Formatted  string `json:"formatted"`
	GivenName  string `json:"givenName"`
}

// TODO(josegomezr): Expand on the property mappings side and here what do we
// want to persist from authentik into LDAP
type UserRequest struct {
	Schemas     []string    `json:"schemas"`
	Active      bool        `json:"active"`
	Emails      []UserEmail `json:"emails"`
	ExternalId  string      `json:"externalId"`
	Name        UserName    `json:"name"`
	DisplayName string      `json:"displayName"`
	UserName    string      `json:"userName"`
}

// TODO(josegomezr): Expand on the property mappings side and here what do we
// want to persist from authentik into LDAP
type GroupRequest struct {
	Schemas               []string `json:"schemas"`
	ExternalId            string   `json:"externalId"`
	LDAPDistinguishedName UserName `json:"ldapDistinguishedName"`
	DisplayName           string   `json:"displayName"`
}

type UserRespOK struct {
	Id         string `json:"id"`
	ExternalId string `json:"externalId"`
}

type UserFilterResponse struct {
	Resources []UserRespOK `json:"Resources"`
}
