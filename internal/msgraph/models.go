package msgraph

type User struct {
	Id                    string `json:"id,omitempty"`
	GivenName             string `json:"givenName,omitempty"`
	Surname               string `json:"surname,omitempty"`
	DisplayName           string `json:"displayName,omitempty"`
	MailNickname          string `json:"mailNickname,omitempty"`
	UserPrincipalName     string `json:"userPrincipalName,omitempty"`
	AccountEnabled        bool   `json:"accountEnabled"`
	OnPremisesImmutableId string `json:"onPremisesImmutableId,omitempty"`
}

type UserListResponse struct {
	Count    int    `json:"@odata.count"`
	NextLink string `json:"@odata.nextLink"`
	Users    []User `json:"value"`
}

type AuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}
