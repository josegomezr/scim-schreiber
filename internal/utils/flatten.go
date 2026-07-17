package utils

import (
	"fmt"
	"strings"
)

// Retrieve extension attribute independent based on the two styles possible.
// See also FlattenPatchMiddleware
// https://github.com/elimity-com/scim/issues/204
func GetExtensionAttribute(attributes map[string]interface{}, extension string, attributeName string) (interface{}, bool) {
	if ext, ok := attributes[extension].(map[string]interface{}); ok {
		val, ok := ext[attributeName]
		return val, ok
	} else if value, ok := attributes[fmt.Sprintf("%s:%s", extension, attributeName)]; ok {
		return value, true
	}

	return nil, false
}

func GetOptionalExtensionAttributeValues(attributes map[string]interface{}, extension string, attributeName string) []string {
	value, ok := GetExtensionAttribute(attributes, extension, attributeName)
	if !ok {
		return []string{}
	}
	return AttributeToSlice[string](value)
}

func GetOptionalExtensionAttributeValue(attributes map[string]interface{}, extension string, attributeName string) []string {
	value, ok := GetExtensionAttribute(attributes, extension, attributeName)
	if !ok {
		return []string{}
	}

	return []string{value.(string)}
}

// FlattenAttrs expands maps of extension attributes to fully qualified keys
func FlattenAttrs(attrs map[string]interface{}) map[string]interface{} {
	flattened := make(map[string]interface{})

	for key, val := range attrs {
		if strings.HasPrefix(key, "urn:") {
			if nestedMap, ok := val.(map[string]interface{}); ok {
				for subKey, subVal := range nestedMap {
					flatKey := fmt.Sprintf("%s:%s", key, subKey)
					flattened[flatKey] = subVal
				}
				continue
			}
		}

		flattened[key] = val
	}

	return flattened
}
