package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/josegomezr/scim-schreiber-ldap/internal/utils"
)

// FlattenPatchMiddleware will modify PATCH requests through utils.FlattenAttrs
// https://github.com/elimity-com/scim/issues/204
func FlattenPatchMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// flattening is only applicable to PATCH
		if r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)

			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)

			return
		}
		_ = r.Body.Close()

		var patchReq map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &patchReq); err != nil {
			// invalid JSON, restore original body
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			next.ServeHTTP(w, r)

			return
		}

		if ops, ok := patchReq["Operations"].([]interface{}); ok {
			for _, opIntf := range ops {
				if op, ok := opIntf.(map[string]interface{}); ok {
					// flattening is only applicable to operations with "value" and without "path"
					_, hasPath := op["path"]
					val, hasValue := op["value"]

					if !hasPath && hasValue {
						if valMap, ok := val.(map[string]interface{}); ok {
							op["value"] = utils.FlattenAttrs(valMap)
						}
					}
				}
			}
		}

		newBody, err := json.Marshal(patchReq)
		if err != nil {
			http.Error(w, "Failed to encode modified request body", http.StatusInternalServerError)
			return
		}

		r.Body = io.NopCloser(bytes.NewBuffer(newBody))
		r.ContentLength = int64(len(newBody))

		next.ServeHTTP(w, r)
	})
}
