package main

import (
	"encoding/json"
	"fmt"
	ldap "github.com/go-ldap/ldap/v3"
	"github.com/josegomezr/scim-schreiber-ldap/internal/scim"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
)

type LDAPHandler struct {
	conn *ldap.Conn
	cfg  *Config
}

func (l *LDAPHandler) ConnectMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := setupLdap()
		if err != nil {
			w.WriteHeader(500)
			return
		}
		defer conn.Close()
		l.conn = conn
		next.ServeHTTP(w, r)
	})
}

func (l *LDAPHandler) ServiceProviderConfigHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("GET /v2/ServiceProviderConfig")
	json.NewEncoder(w).Encode(NewServiceProviderConfig())
}

func (l *LDAPHandler) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	obj := scim.UserRequest{}
	err := json.NewDecoder(r.Body).Decode(&obj)
	slog.Info("POST /v2/Users")
	if err != nil {
		w.WriteHeader(400)
		return
	}

	slog.Info("POST /v2/Users", "request", obj)
	if searchUser(l.conn, obj.UserName) != nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	if !l.cfg.AllowUserCreation {
		w.WriteHeader(http.StatusForbidden)
	} else {
		// Not implemented.
		w.WriteHeader(http.StatusNotAcceptable)
	}
}
func (l *LDAPHandler) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	slog.Info("GET /v2/Users", "qs", qs)

	filter := qs.Get("filter")
	if filter != "" && !strings.HasPrefix(filter, `userName eq "`) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "{}")
		return
	}

	uid := "*"
	if filter != "" {
		uid = filter[13 : len(filter)-1]
	}

	resp := scim.UserFilterResponse{}
	resp.Resources = make([]scim.UserRespOK, 0)

	users, err := searchUsers(l.conn, uid, "uid")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "{}")
	}

	//  TODO: Implement pagination here.
	for entry, err := range users {
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "{}")
			return
		}

		resp.Resources = append(resp.Resources, *ldapEntryToUserResponse(entry))
	}

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
	return
}

func (l *LDAPHandler) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	userUuid := r.PathValue("user_uuid")
	slog.Info("GET /v2/Users/", "userUuid", userUuid)

	users, err := searchUsers(l.conn, userUuid, "uuid")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "{}")
	}

	//  TODO: Implement pagination here.
	for entry, err := range users {
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "{}")
			return
		}

		w.WriteHeader(200)
		json.NewEncoder(w).Encode(ldapEntryToUserResponse(entry))
		return
	}

	w.WriteHeader(404)
}
func (l *LDAPHandler) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.PathValue("user_id")
	obj := scim.UserRequest{}
	err := json.NewDecoder(r.Body).Decode(&obj)
	slog.Info("PUT /v2/Users/", "userId", userId, "obj", obj)
	if err != nil {
		w.WriteHeader(401)
		fmt.Fprintln(w, "{}")
		return
	}

	if entry := searchUserByUUID(l.conn, userId); entry != nil {
		slog.Info("Found user", "entry", entry)

		// TODO(josegomezr): change more details
		slog.Info("Updating user details.", "from", entry.GetAttributeValue("cn"), "to", obj.DisplayName)
		replaces := map[string][]string{
			"cn":           []string{obj.DisplayName},
			"sshPublicKey": obj.SshPublicKeys,
		}
		err := updateEntry(l.conn, entry.DN, nil, nil, replaces)

		if err != nil {
			slog.Error("Error updating entry", "error", err)
			w.WriteHeader(401)
			return
		}

		w.WriteHeader(200)
		json.NewEncoder(w).Encode(ldapEntryToUserResponse(entry))
		return
	}
	w.WriteHeader(404)
	fmt.Fprintln(w, "{}")
}

// TODO(josegomezr): sometimes IDP's don't really _delete_ things. We gotta find a creative way to
//
//	                  issue a deletion from SCIM.
//						 Model is set to be deleted, task to roll updates is scheduled in the background
//	                  by the time the task is executed, the record does not exist in the DB anymore.
func (l *LDAPHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userId := r.PathValue("user_id")
	fmt.Println("DELETE /v2/Users", userId)
	if u := searchUserByUUID(l.conn, userId); u != nil {
		// delete the user
		return
	}
	// in either case (Whether it exists or not we return 200)
	w.WriteHeader(200)
	fmt.Fprintln(w, "{}")
}

// TODO(josegomezr): Groups don't self-heal after an 409 Conflict like
//
//					     users, we gotta make it behave like an UPSERT rather
//	                  than a POST+GET (as users do).
//	                  Prolly make sense to make group & users behave like an UPSERT
func (l *LDAPHandler) CreateGroupHandler(w http.ResponseWriter, r *http.Request) {
	obj := scim.GroupRequest{}
	err := json.NewDecoder(r.Body).Decode(&obj)
	slog.Info("POST /v2/Groups", "obj", obj)

	if err != nil {
		fmt.Println("post-group:", err)
		w.WriteHeader(400)
		fmt.Fprintln(w, "bad json")
		return
	}

	entries, err := searchGroups(l.conn, obj.DisplayName, "cn")
	if err != nil {
		w.WriteHeader(400)
		return
	}

	for entry, err := range entries {
		if err != nil {
			w.WriteHeader(400)
			return
		}

		if l.cfg.GroupCreationIsUpsert {
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(ldapEntryToGroupResponse(entry))
			return
		} else {
			w.WriteHeader(400)
			return
		}
	}

	w.WriteHeader(401)
	return
}

func (l *LDAPHandler) GetGroupHandler(w http.ResponseWriter, r *http.Request) {
	groupId := r.PathValue("group_id")
	slog.Info("GET /v2/Groups", "groupId", groupId)

	entries, err := searchGroups(l.conn, groupId, "cn")
	if err != nil {
		w.WriteHeader(400)
		return
	}

	for entry, err := range entries {
		if err != nil {
			w.WriteHeader(400)
			return
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ldapEntryToGroupResponse(entry))
		return
	}

	w.WriteHeader(404)
}

func (l *LDAPHandler) UpdateGroupHandler(w http.ResponseWriter, r *http.Request) {
	groupId := r.PathValue("group_id")
	slog.Info("PATCH /v2/Groups", "groupId", groupId)

	obj := scim.PatchRequest{}
	err := json.NewDecoder(r.Body).Decode(&obj)
	if err != nil {
		w.WriteHeader(400)
		fmt.Fprintln(w, "{}")
		return
	}
	slog.Info("PATCH /v2/Groups", "obj", obj)

	entries, err := searchGroups(l.conn, groupId, "cn")
	if err != nil {
		w.WriteHeader(400)
		return
	}

	for entry, err := range entries {
		if err != nil {
			w.WriteHeader(400)
			return
		}

		adds, removes, replaces := classifyPatchOperations(obj.Operations)
		l.transformMemberUUIDtoUID(adds, "members", "memberUid")
		l.transformMemberUUIDtoUID(removes, "members", "memberUid")

		err := updateEntry(l.conn, entry.DN, adds, removes, replaces)

		if err != nil {
			w.WriteHeader(401)
			return
		}

		w.WriteHeader(200)
		fmt.Fprintf(w, "{}")
		return
	}

	w.WriteHeader(404)
	fmt.Fprintln(w, "{}")
}

// TODO(josegomezr): Sometimes IDP's don't really _delete_ things. We gotta find a creative way to
//
//	                  issue a deletion from SCIM.
//						 Model is set to be deleted, task to roll updates is scheduled in the background
//	                  by the time the task is executed, the record does not exist in the DB anymore.
func (l *LDAPHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	userId := r.PathValue("user_id")
	fmt.Println("DELETE /v2/Groups", userId)
	if u := searchUserByUUID(l.conn, userId); u != nil {
		// delete the user
		return
	}
	// in either case (Whether it exists or not we return 200)
	w.WriteHeader(200)
	fmt.Fprintln(w, "{}")
}

func (l *LDAPHandler) transformMemberUUIDtoUID(attrmap map[string][]string, source string, dest string) {
	if attrmap[source] == nil {
		return
	}

	if attrmap[dest] == nil {
		attrmap[dest] = make([]string, 0)
	}

	for _, userUuid := range attrmap[source] {
		if lookup := searchUserByUUID(l.conn, userUuid); lookup != nil {
			attrmap[dest] = append(attrmap[dest], lookup.GetAttributeValue("uid"))
		}
	}

	delete(attrmap, source)

}

func classifyPatchOperations(operations []scim.PatchOp) (map[string][]string, map[string][]string, map[string][]string) {
	adds := make(map[string][]string)
	removes := make(map[string][]string)
	replaces := make(map[string][]string)

	for _, op := range operations {
		if reflect.ValueOf(op.Value).Kind() == reflect.Slice {
			for _, singleVal := range CastMultiValue[map[string]interface{}](op.Value) {
				value := CastSingleValue[string](singleVal["value"])

				switch op.Operation {
				case "add":
					adds[op.Path] = append(adds[op.Path], value)
				case "remove":
					removes[op.Path] = append(removes[op.Path], value)
				default:
					slog.Info("unknown OP", "operation", op.Operation, "path", op.Path, "value", singleVal)
					continue
				}
			}
		} else {
			switch op.Operation {
			case "add":
				adds[op.Path] = append(adds[op.Path], fmt.Sprintf("%s", op.Value))
			case "remove":
				removes[op.Path] = append(removes[op.Path], fmt.Sprintf("%s", op.Value))
			case "replace":
				for k, v := range op.Value.(map[string]interface{}) {
					replaces[k] = append(replaces[k], fmt.Sprintf("%s", v))
				}
			default:
				slog.Info("unknown OP", "operation", op.Operation, "path", op.Path, "value", op.Value)
				continue
			}
		}
	}

	return adds, removes, replaces
}
