package utils

import "github.com/elimity-com/scim"

func AttributeToSlice[T any](value interface{}) []T {
	list := value.([]interface{})

	result := make([]T, 0, len(list))

	for _, entry := range list {
		result = append(result, entry.(T))
	}

	return result
}

func GetOptionalAttribute(attributes scim.ResourceAttributes, name string) []string {
	value, ok := attributes[name]

	if !ok {
		return []string{}
	}

	return []string{value.(string)}
}

func GetOptionalSingleAttribute(attributes map[string]interface{}, name string) string {
	value, ok := attributes[name]

	if !ok {
		return ""
	}

	return value.(string)
}

type Phones struct {
	Work   string
	Mobile string
}

func (p Phones) WorkLdap() []string {
	if p.Work != "" {
		return []string{p.Work}
	}
	return []string{}
}

func (p Phones) MobileLdap() []string {
	if p.Mobile != "" {
		return []string{p.Mobile}
	}
	return []string{}
}

func GetPhones(attributes map[string]interface{}) Phones {
	phones := Phones{}

	if telephones, ok := attributes["phoneNumbers"]; ok {
		for _, phone := range telephones.([]interface{}) {
			phoneEntry := phone.(map[string]interface{})
			phoneNr, ok := phoneEntry["value"].(string)
			if !ok {
				continue
			}
			phoneType, ok := phoneEntry["type"].(string)
			if !ok {
				continue
			}
			switch phoneType {
			case "work":
				phones.Work = phoneNr
				break
			case "mobile":
				phones.Mobile = phoneNr
				break
			}
		}
	}

	return phones
}
