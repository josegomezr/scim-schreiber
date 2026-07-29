package main

import (
	"encoding/json"
	"log/slog"
	"reflect"
	"slices"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"
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
	Value bool `json:"isSupervisor"`
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

type CustomField struct {
	Name        string
	DisplayName string
	MultiValued bool
	FieldType   string
}

type CustomSchema struct {
	Name        string
	DisplayName string
	Fields      []CustomField
}

// This is a complete legacy spec created by GWS application owners.
// Which is why each field has it's own schema instead of one schema with multiple fields.
func CustomSchemaSpec() []CustomSchema {
	return []CustomSchema{
		{
			Name:        "Region7281",
			DisplayName: "Region",
			Fields: []CustomField{

				{
					Name:        "Office",
					DisplayName: "Office",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "User1921",
			DisplayName: "User",
			Fields: []CustomField{

				{
					Name:        "isSupervisor",
					DisplayName: "isSupervisor",
					MultiValued: false,
					FieldType:   "BOOL",
				},
			},
		},

		{
			Name:        "ProxyAddresses",
			DisplayName: "ProxyAddresses",
			Fields: []CustomField{

				{
					Name:        "ProxyAddresses",
					DisplayName: "ProxyAddresses",
					MultiValued: true,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "Region",
			DisplayName: "Region",
			Fields: []CustomField{

				{
					Name:        "addressesWorkCountryCode",
					DisplayName: "addressesWorkCountryCode",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "Senior_Leader",
			DisplayName: "Senior Leader",
			Fields: []CustomField{

				{
					Name:        "senior_leaders",
					DisplayName: "secondaryWorkforceManagerID",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "User",
			DisplayName: "User",
			Fields: []CustomField{

				{
					Name:        "userType",
					DisplayName: "userType",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "User2471",
			DisplayName: "User",
			Fields: []CustomField{

				{
					Name:        "workLocationType",
					DisplayName: "workLocationType",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "Status",
			DisplayName: "Status",
			Fields: []CustomField{

				{
					Name:        "User_status",
					DisplayName: "User status",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "Job_Family",
			DisplayName: "Job Family",
			Fields: []CustomField{

				{
					Name:        "jobfamily5241",
					DisplayName: "jobfamily",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "L3_Leader",
			DisplayName: "L3 Leader",
			Fields: []CustomField{

				{
					Name:        "L3_Leader",
					DisplayName: "userLeader",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},

		{
			Name:        "User7951",
			DisplayName: "User",
			Fields: []CustomField{

				{
					Name:        "division",
					DisplayName: "division",
					MultiValued: false,
					FieldType:   "STRING",
				},
			},
		},
	}
}

func SyncSchemas(adminClient *admin.Service) error {
	schemas, err := adminClient.Schemas.List("my_customer").Do()

	if err != nil {
		return err
	}

	customSchemas := CustomSchemaSpec()

	for _, customSchema := range customSchemas {
		slog.Info("Syncing schema", "schema", customSchema.Name)

		fields := make([]*admin.SchemaFieldSpec, 0, len(customSchema.Fields))

		for _, field := range customSchema.Fields {
			fields = append(fields, &admin.SchemaFieldSpec{
				DisplayName:    field.DisplayName,
				FieldName:      field.Name,
				FieldType:      field.FieldType,
				MultiValued:    field.MultiValued,
				ReadAccessType: "ADMINS_AND_SELF",
			})
		}

		newSchema := admin.Schema{
			DisplayName: customSchema.DisplayName,
			SchemaName:  customSchema.Name,
			Fields:      fields,
		}

		schemaExists := slices.ContainsFunc(schemas.Schemas, func(s *admin.Schema) bool {
			return strings.EqualFold(s.SchemaName, customSchema.Name)
		})

		if schemaExists {
			_, err := adminClient.Schemas.Update("my_customer", customSchema.Name, &newSchema).Do()
			if err != nil {
				return err
			}
		} else {
			_, err := adminClient.Schemas.Insert("my_customer", &newSchema).Do()

			if err != nil {
				return err
			}
		}
	}
	return nil
}
