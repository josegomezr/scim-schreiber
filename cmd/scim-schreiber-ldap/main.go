package main

// TODO(josegomezr): use SLOG + json
import (
	"encoding/json"
	"fmt"
	ldap "github.com/go-ldap/ldap/v3"
	"github.com/josegomezr/scim-schreiber-ldap/internal/scim"
	"iter"
	"log"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
)

func CastSingleValue[T any](input interface{}) T {
	output, _ := input.(T)
	return output
}

func CastMultiValue[T any](input interface{}) []T {
	var out []T
	for _, i := range input.([]interface{}) {
		out = append(out, CastSingleValue[T](i))
	}
	return out
}

// TODO(josegomezr): Move these DTO into response types
type User struct {
	Uid         string
	Dn          string
	Uuid        string
	DisplayName string
}

type Group struct {
	Dn          string
	DisplayName string
}

func ldapEntryToUserResponse(entry *ldap.Entry) *scim.UserRespOK {
	return &scim.UserRespOK{
		ExternalId: entry.DN,
		Id:         entry.GetAttributeValue("uuid"),
	}
}

func ldapEntryToGroupResponse(entry *ldap.Entry) *scim.GroupRespOK {
	members := []scim.ValueObj{}
	for _, value := range entry.GetAttributeValues("memberUid") {
		members = append(members, scim.ValueObj{
			Value: value,
		})
	}

	// TODO: Id's need something more qualifies, maybe we can use the UUID
	// generated in the IDP
	//
	// If not we should fallback to DN's everywhere.
	return &scim.GroupRespOK{
		Id:          entry.GetAttributeValue("cn"),
		ExternalId:  entry.DN,
		DisplayName: entry.GetAttributeValue("cn"),
		Members:     members,
	}
}

func searchGroups(uid string, field string) (iter.Seq2[*ldap.Entry, error], error) {
	filter := fmt.Sprintf("(%s=%s)", field, uid)

	baseUid := os.Getenv("LDAP_BASE_GROUP_OU") + "," + os.Getenv("LDAP_BASE_DN")
	return ldapSearchIter(LDAP_CONN, filter, baseUid), nil
}

func searchUsers(uid string, field string) (iter.Seq2[*ldap.Entry, error], error) {
	filter := fmt.Sprintf("(%s=%s)", field, uid)

	baseUid := os.Getenv("LDAP_BASE_USER_OU") + "," + os.Getenv("LDAP_BASE_DN")
	return ldapSearchIter(LDAP_CONN, filter, baseUid), nil
}

func _searchUser(uid string, field string) *ldap.Entry {
	users, err := searchUsers(uid, field)
	if err != nil {
		return nil
	}

	for user, err := range users {
		if err != nil {
			return nil
		}
		return user
	}

	return nil
}

func searchUser(uid string) *ldap.Entry {
	return _searchUser(uid, "uid")
}

func searchUserByUUID(uid string) *ldap.Entry {
	return _searchUser(uid, "uuid")
}

type Config struct {
	AllowUserCreation     bool
	GroupCreationIsUpsert bool
}

func main() {
	ldapEndpoint := os.Getenv("LDAP_URL")
	ldapBindDn := os.Getenv("LDAP_BIND_DN")
	ldapBindPw := os.Getenv("LDAP_BIND_PW")

	cfg := Config{
		AllowUserCreation:     false,
		GroupCreationIsUpsert: true,
	}
	mux := http.NewServeMux()

	conn, err := connectAndBind(ldapEndpoint, ldapBindDn, ldapBindPw)
	if err != nil {
		log.Fatalf("BORKED, LDAP NOT WORKING: %s", err)
	}

	LDAP_CONN = conn

	mux.HandleFunc("GET /-/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.HandleFunc("GET /v2/ServiceProviderConfig", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("GET /v2/ServiceProviderConfig")
		json.NewEncoder(w).Encode(NewServiceProviderConfig())
	})

	// TODO(josegomezr): Split handlers into separate functions
	mux.HandleFunc("POST /v2/Users", func(w http.ResponseWriter, r *http.Request) {
		obj := scim.UserRequest{}
		err := json.NewDecoder(r.Body).Decode(&obj)
		slog.Info("POST /v2/Users")
		if err != nil {
			w.WriteHeader(400)
			return
		}

		slog.Info("POST /v2/Users", "request", obj)
		if searchUser(obj.UserName) != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}

		if !cfg.AllowUserCreation {
			w.WriteHeader(http.StatusForbidden)
		} else {
			// Not implemented.
			w.WriteHeader(http.StatusNotAcceptable)
		}
	})

	mux.HandleFunc("GET /v2/Users", func(w http.ResponseWriter, r *http.Request) {
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

		users, err := searchUsers(uid, "uid")
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
	})

	mux.HandleFunc("GET /v2/Users/{user_uuid}", func(w http.ResponseWriter, r *http.Request) {
		userUuid := r.PathValue("user_uuid")
		slog.Info("GET /v2/Users/", "userUuid", userUuid)

		users, err := searchUsers(userUuid, "uuid")
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
	})

	mux.HandleFunc("PUT /v2/Users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		userId := r.PathValue("user_id")
		obj := scim.UserRequest{}
		err := json.NewDecoder(r.Body).Decode(&obj)
		slog.Info("PUT /v2/Users/%s", userId, "obj", obj)
		if err != nil {
			w.WriteHeader(401)
			fmt.Fprintln(w, "{}")
			return
		}

		if entry := searchUserByUUID(userId); entry != nil {
			slog.Info("Found user", "entry", entry)
			// TODO(josegomezr): change more details
			slog.Info("Updating user details. from=%q to=%q\n", entry.GetAttributeValue("cn"), obj.DisplayName)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(ldapEntryToUserResponse(entry))
			return
		}
		w.WriteHeader(404)
		fmt.Fprintln(w, "{}")
	})

	// TODO(josegomezr): sometimes IDP's don't really _delete_ things. We gotta find a creative way to
	//                   issue a deletion from SCIM.
	// 					 Model is set to be deleted, task to roll updates is scheduled in the background
	//                   by the time the task is executed, the record does not exist in the DB anymore.
	mux.HandleFunc("DELETE /v2/Users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		userId := r.PathValue("user_id")
		fmt.Println("DELETE /v2/Users", userId)
		if u := searchUserByUUID(userId); u != nil {
			// delete the user
			return
		}
		// in either case (Whether it exists or not we return 200)
		w.WriteHeader(200)
		fmt.Fprintln(w, "{}")
	})

	// TODO(josegomezr): Groups don't self-heal after an 409 Conflict like
	// 				     users, we gotta make it behave like an UPSERT rather
	//                   than a POST+GET (as users do).
	//                   Prolly make sense to make group & users behave like an UPSERT
	mux.HandleFunc("POST /v2/Groups", func(w http.ResponseWriter, r *http.Request) {
		obj := scim.GroupRequest{}
		err := json.NewDecoder(r.Body).Decode(&obj)
		slog.Info("POST /v2/Groups", "obj", obj)

		if err != nil {
			fmt.Println("post-group:", err)
			w.WriteHeader(400)
			fmt.Fprintln(w, "bad json")
			return
		}

		entries, err := searchGroups(obj.DisplayName, "cn")
		if err != nil {
			w.WriteHeader(400)
			return
		}

		for entry, err := range entries {
			if err != nil {
				w.WriteHeader(400)
				return
			}

			if cfg.GroupCreationIsUpsert {
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
	})

	mux.HandleFunc("GET /v2/Groups/{group_id}", func(w http.ResponseWriter, r *http.Request) {
		groupId := r.PathValue("group_id")
		slog.Info("GET /v2/Groups", "groupId", groupId)

		entries, err := searchGroups(groupId, "cn")
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
	})

	mux.HandleFunc("PATCH /v2/Groups/{group_id}", func(w http.ResponseWriter, r *http.Request) {
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

		entries, err := searchGroups(groupId, "cn")
		if err != nil {
			w.WriteHeader(400)
			return
		}

		for entry, err := range entries {
			if err != nil {
				w.WriteHeader(400)
				return
			}

			adds := make(map[string][]string)
			removes := make(map[string][]string)
			replaces := make(map[string]string)

			for _, op := range obj.Operations {
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
							replaces[k] = fmt.Sprintf("%s", v)
						}
					default:
						slog.Info("unknown OP", "operation", op.Operation, "path", op.Path, "value", op.Value)
						continue
					}
				}
			}

			transformMemberUUIDtoUID(adds["members"])
			transformMemberUUIDtoUID(removes["members"])

			err := updateEntry(entry.DN, adds, removes, replaces)

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
	})

	// TODO(josegomezr): Sometimes IDP's don't really _delete_ things. We gotta find a creative way to
	//                   issue a deletion from SCIM.
	// 					 Model is set to be deleted, task to roll updates is scheduled in the background
	//                   by the time the task is executed, the record does not exist in the DB anymore.
	mux.HandleFunc("DELETE /v2/Groups/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		userId := r.PathValue("user_id")
		fmt.Println("DELETE /v2/Groups", userId)
		if u := searchUserByUUID(userId); u != nil {
			// delete the user
			return
		}
		// in either case (Whether it exists or not we return 200)
		w.WriteHeader(200)
		fmt.Fprintln(w, "{}")
	})

	fmt.Println("Listening")
	// TODO(josegomezr): configurable ports here
	http.ListenAndServe(":9440", mux)
}

func transformMemberUUIDtoUID(uuids []string) {
	if uuids == nil {
		return
	}

	for k, userUuid := range uuids {
		if lookup := searchUserByUUID(userUuid); lookup != nil {
			uuids[k] = lookup.GetAttributeValue("uid")
		}
	}
}

func updateEntry(dn string, adds map[string][]string, removes map[string][]string, replaces map[string]string) error {
	endpoint := os.Getenv("LDAP_URL")
	bindDn := os.Getenv("LDAP_BIND_DN")
	bindPw := os.Getenv("LDAP_BIND_PW")

	request := ldap.NewModifyRequest(dn, nil)
	for k, vals := range adds {
		request.Add(k, vals)
	}
	for k, vals := range removes {
		request.Delete(k, vals)
	}
	for k, v := range replaces {
		request.Replace(k, []string{v})
	}

	return LDAP_CONN.Modify(request)
}
