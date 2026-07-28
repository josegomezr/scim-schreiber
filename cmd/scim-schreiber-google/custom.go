package main

import (
	"encoding/json"
	"reflect"
	"strings"

	"google.golang.org/api/googleapi"
)

type JobFamily struct {
	Value string `json:"jobfamily5241,omitempty"`
}

type L3Leader struct {
	Value string `json:"L3_Leader,omitempty"`
}

type ProxyAddress struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

type ProxyAddresses struct {
	ProxyAddresses []ProxyAddress `json:"ProxyAddresses,omitempty"`
}

type Region struct {
	CountryCode string `json:"addressesWorkCountryCode,omitempty"`
}

type Office struct {
	Value string `json:"Office,omitempty"`
}

type UserType struct {
	Value string `json:"userType,omitempty"`
}

type IsSupervisor struct {
	Value bool `json:"isSupervisor,omitempty"`
}

type WorkLocationType struct {
	Value string `json:"workLocationType,omitempty"`
}

type Division struct {
	Value string `json:"division,omitempty"`
}

type CustomFields struct {
	JobFamily        JobFamily        `json:"Job_Family,omitempty"`
	L3Leader         L3Leader         `json:"L3_Leader,omitempty"`
	ProxyAddresses   ProxyAddresses   `json:"ProxyAddresses,omitempty"`
	Region           Region           `json:"Region,omitempty"`
	Office           Office           `json:"Region7281,omitempty"`
	UserType         UserType         `json:"User,omitempty"`
	IsSupervisor     IsSupervisor     `json:"User1921,omitempty"`
	WorkLocationType WorkLocationType `json:"User2471,omitempty"`
	Division         Division         `json:"User7951,omitempty"`
}

func UnmarshalGoogleApi(data map[string]googleapi.RawMessage) (CustomFields, error) {
	c := CustomFields{}

	value := reflect.ValueOf(&c).Elem()
	typeOfT := value.Type()

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)

		jsonTag := typeOfT.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(jsonTag, ",")

		if bytes, ok := data[name]; ok {
			v := reflect.New(typeOfT.Field(i).Type)
			err := json.Unmarshal(bytes, v.Interface())
			if err != nil {
				return CustomFields{}, err
			}

			field.Set(v.Elem())
		}
	}

	return c, nil
}

func (c CustomFields) MarshalGoogleApi() (map[string]googleapi.RawMessage, error) {
	value := reflect.ValueOf(&c).Elem()
	typeOfT := value.Type()

	ret := make(map[string]googleapi.RawMessage, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)

		bytes, err := json.Marshal(field.Interface())

		if err != nil {
			return nil, err
		}

		jsonTag := typeOfT.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(jsonTag, ",")

		ret[name] = bytes
	}

	return ret, nil
}
