package main

// TODO(josegomezr): use SLOG + json
import (
	"encoding/json"
	"fmt"
	"github.com/josegomezr/scim-schreiber-ldap/internal/scim"
	"io"
	"net/http"
	"os"
	"strings"
)

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

func searchGroup(cn string) *Group {
	users, err := connectAndSearch(fmt.Sprintf("cn=%s", cn))
	if err != nil {
		return nil
	}

	if len(users) < 1 {
		return nil
	}

	// TODO(josegomezr): Align with Group DTO
	return &Group{
		Dn:          users[0].DN,
		DisplayName: users[0].GetAttributeValue("cn"),
	}
}

func _searchUser(uid string, field string) *User {
	users, err := connectAndSearch(fmt.Sprintf("%s=%s", field, uid))
	if err != nil {
		return nil
	}

	if len(users) < 1 {
		return nil
	}

	// TODO(josegomezr): Align with User DTO
	return &User{
		Uid:         users[0].GetAttributeValue("uid"),
		Dn:          users[0].DN,
		Uuid:        users[0].GetAttributeValue("uuid"),
		DisplayName: users[0].GetAttributeValue("cn"),
	}
}

func searchUser(uid string) *User {
	return _searchUser(uid, "uid")
}

func searchUserByUUID(uid string) *User {
	return _searchUser(uid, "uuid")
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /-/startup", func(w http.ResponseWriter, r *http.Request) {
		// TODO(josegomezr): Test LDAP Connectivity here
		w.WriteHeader(200)
	})

	mux.HandleFunc("GET /-/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mux.HandleFunc("GET /v2/ServiceProviderConfig", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("GET /v2/ServiceProviderConfig")
		json.NewEncoder(w).Encode(NewServiceProviderConfig())
	})

	// TODO(josegomezr): Split handlers into separate functions

	mux.HandleFunc("POST /v2/Users", func(w http.ResponseWriter, r *http.Request) {
		obj := scim.UserRequest{}
		err := json.NewDecoder(r.Body).Decode(&obj)
		fmt.Printf("POST /v2/Users %+v\n", obj)
		if err != nil {
			w.WriteHeader(400)
			fmt.Fprintln(w, "bad json")
			return
		}

		if searchUser(obj.UserName) != nil {
			w.WriteHeader(409)
			return
		}

		fmt.Printf("Creating (or maybe updating?) User by request: %+v\n", obj)

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(scim.UserRespOK{
			ExternalId: fmt.Sprintf("uid=id-for-%s", obj.UserName),
			Id:         obj.ExternalId,
		})
	})

	mux.HandleFunc("GET /v2/Users", func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		fmt.Printf("GET /v2/Users %+v\n", qs)

		filter := qs.Get("filter")
		if filter != "" && !strings.HasPrefix(filter, `userName eq "`) {
			w.WriteHeader(400)
			fmt.Fprintln(w, "{}")
			return
		}

		if filter == "" {
			// TODO: don't be lazy, do things well...
			filter = `userName eq "*"`
		}

		uid := filter[13 : len(filter)-1]

		resp := scim.UserFilterResponse{}
		resp.Resources = make([]scim.UserRespOK, 0)

		if u := searchUser(uid); u != nil {
			resp.Resources = append(resp.Resources, scim.UserRespOK{
				ExternalId: u.Dn,
				Id:         u.Uuid,
			})
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(resp)
		return
	})

	mux.HandleFunc("PUT /v2/Users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		userId := r.PathValue("user_id")
		obj := scim.UserRequest{}
		err := json.NewDecoder(r.Body).Decode(&obj)
		fmt.Printf("PUT /v2/Users/%s %+v\n", userId, obj)
		if err != nil {
			w.WriteHeader(401)
			fmt.Fprintln(w, "{}")
			return
		}

		if u := searchUserByUUID(userId); u != nil {
			fmt.Printf("Found user=%+v\n", u)
			// TODO(josegomezr): change more details
			fmt.Printf("Updating user details. from=%q to=%q\n", u.DisplayName, obj.DisplayName)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(scim.UserRespOK{
				ExternalId: u.Dn,
				Id:         u.Uid,
			})
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
		fmt.Printf("POST /v2/Groups %+v\n", obj)

		if err != nil {
			fmt.Println("post-group:", err)
			w.WriteHeader(400)
			fmt.Fprintln(w, "bad json")
			return
		}

		if searchGroup(obj.DisplayName) != nil {
			w.WriteHeader(409)
			return
		}

		// TODO(josegomezr): write 2xx responses when the previous behavior works
		// w.WriteHeader(201)
		// json.NewEncoder(w).Encode(scim.UserRespOK{
		// 	ExternalId: fmt.Sprintf("uid=id-for-%s", obj.UserName),
		// 	Id: obj.ExternalId,
		// })
	})

	mux.HandleFunc("GET /v2/Groups", func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		fmt.Printf("GET /v2/Groups %+v\n", qs)

		filter := qs.Get("filter")
		if filter != "" && !strings.HasPrefix(filter, `userName eq "`) {
			w.WriteHeader(400)
			fmt.Fprintln(w, "{}")
			return
		}

		if filter == "" {
			// TODO: don't be lazy, do things well...
			filter = `userName eq "*"`
		}

		uid := filter[13 : len(filter)-1]

		resp := scim.UserFilterResponse{}
		resp.Resources = make([]scim.UserRespOK, 0)

		if u := searchUser(uid); u != nil {
			resp.Resources = append(resp.Resources, scim.UserRespOK{
				ExternalId: u.Dn,
				Id:         u.Uuid,
			})
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(resp)
		return
	})

	mux.HandleFunc("PUT /v2/Groups/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		userId := r.PathValue("user_id")
		fmt.Printf("PUT /v2/Groups/%s\n", userId)
		// TODO(josegomezr): Group membership comes PUT/PATCH ops that are not handled yet.
		io.Copy(os.Stdout, r.Body)

		// if u := searchUserByUUID(userId); u != nil {
		// 	fmt.Println("Found user=", userId)
		// 	w.WriteHeader(200)
		// 	json.NewEncoder(w).Encode(scim.UserRespOK{
		// 		ExternalId: u.Dn,
		// 		Id: u.Uid,
		// 	})
		// 	return
		// }

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
