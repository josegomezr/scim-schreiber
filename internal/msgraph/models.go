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

type Group struct {
	Id              string   `json:"id,omitempty"`
	DisplayName     string   `json:"displayName,omitempty"`
	CreatedDateTime string   `json:"createdDateTime,omitempty"`
	MailEnabled     bool     `json:"mailEnabled"`
	MailNickname    string   `json:"mailNickname"`
	SecurityEnabled bool     `json:"securityEnabled"`
	Members         []Member `json:"members,omitempty"`
}

type Member struct {
	Id                string `json:"id,omitempty"`
	DisplayName       string `json:"displayName,omitempty"`
	UserPrincipalName string `json:"userPrincipalName,omitempty"`
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

type GroupListResponse struct {
	Count    int     `json:"@odata.count"`
	NextLink string  `json:"@odata.nextLink"`
	Groups   []Group `json:"value"`
}

type MemberListResponse struct {
	Count    int      `json:"@odata.count"`
	NextLink string   `json:"@odata.nextLink"`
	Members  []Member `json:"value"`
}

type BindRequest struct {
	ODataID string `json:"@odata.id"`
}
