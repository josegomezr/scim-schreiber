package jira

type User struct {
	Self         string `json:"self,omitempty"`
	Key          string `json:"key,omitempty"`
	Name         string `json:"name,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	Active       bool   `json:"active"`
	Deleted      bool   `json:"deleted,omitempty"`
}

type Group struct {
	Members     []User `json:"members,omitempty"`
	DisplayName string `json:"name,omitempty"`
	Self        string `json:"self,omitempty"`
}

type GroupListResponse struct {
	Total  int     `json:"total"`
	Groups []Group `json:"groups"`
}

type UserListResponse struct {
	Users []User
}

type AddMemberRequest struct {
	UserName string `json:"name"`
}

type MemberListResponse struct {
	Total int    `json:"total"`
	Users []User `json:"values"`
}
