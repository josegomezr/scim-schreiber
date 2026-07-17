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
