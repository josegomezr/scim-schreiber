package model

type UserName struct {
	FamilyName string `json:"familyName"`
	Formatted  string `json:"formatted"`
	GivenName  string `json:"givenName"`
}

type ValueObj struct {
	Value string `json:"value"`
}
