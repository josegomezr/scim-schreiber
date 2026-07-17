package utils

import (
	"fmt"
	"strings"
)

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
