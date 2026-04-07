package jira

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
)

func (c *Client) GetUser(username string) (*User, error) {
	newu := c.config.baseURL.JoinPath("/rest/api/2/user")
	v := newu.Query()
	v.Set("username", username)
	newu.RawQuery = v.Encode()

	userResp := User{}
	resp, err := c.newRequestRoundTrip("GET", newu.String(), nil)
	if err != nil {
		return nil, err
	}

	// dummy not found
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return nil, err
	}
	return &userResp, nil
}

func (c *Client) ListAllUsers(username string) iter.Seq2[*User, error] {
	return func(yield func(*User, error) bool) {
		newu := c.config.baseURL.JoinPath("/rest/api/2/user/search")
		v := newu.Query()
		if username != "" {
			v.Set("username", username)
		} else {
			v.Set("username", ".")
		}

		v.Set("maxResults", "1000")
		newu.RawQuery = v.Encode()

		userResp := []User{}
		n := 0
		foundAtLeastOne := false
		for {
			v.Set("startAt", fmt.Sprintf("%d", n))
			newu.RawQuery = v.Encode()
			uri := newu.String()
			fmt.Println("Request", "url", uri)
			resp, err := c.newRequestRoundTrip("GET", uri, nil)
			if err != nil {
				yield(nil, err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				yield(nil, fmt.Errorf("wrong status code: %d", resp.StatusCode))
				return
			}

			if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
				yield(nil, fmt.Errorf("error deserializing json: %w", err))
			}

			if len(userResp) == 0 {
				break
			}

			for _, jiraUser := range userResp {
				foundAtLeastOne = true
				if !yield(&jiraUser, nil) {
					return
				}
				n += 1
			}
		}

		if !foundAtLeastOne && username != "" {
			user, err := c.GetUser(username)
			if err != nil || user == nil {
				return
			}

			yield(user, nil)
		}
	}
}

func (c *Client) UpdateUser(username string, reqUser User) (*User, error) {
	newu := c.config.baseURL.JoinPath("/rest/api/2/user")
	v := newu.Query()
	v.Set("username", username)
	newu.RawQuery = v.Encode()
	user := reqUser

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(user); err != nil {
		return nil, err
	}

	req, err := c.newRequest("PUT", newu.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return nil, fmt.Errorf("Error updating user id=%s, status=%d. details=%s", username, resp.StatusCode, buf.String())
	}

	if user.UserName == "" {
		username = user.UserName
	}

	return c.GetUser(username)
}

var UserAlreadyExists error = errors.New("User with this username already exists.")

func (c *Client) CreateUser(user User) (*User, error) {
	for _, err := range c.ListAllUsers(user.UserName) {
		if err != nil {
			return nil, err
		}
		return nil, UserAlreadyExists
	}

	newUser := user // Shallow copy
	newu := c.config.baseURL.JoinPath("/rest/api/2/user")

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(newUser); err != nil {
		return nil, err
	}

	req, err := c.newRequest("POST", newu.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return nil, fmt.Errorf("Could not create user. Details=%s", buf.String())
	}

	userResp := User{}
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return nil, err
	}
	selfUrl := newu.JoinPath("")
	qs := selfUrl.Query()
	qs.Set("username", newUser.UserName)
	selfUrl.RawQuery = qs.Encode()
	newUser.Key = userResp.Key
	newUser.Self = selfUrl.String()
	return &newUser, nil
}

func (c *Client) DeleteUser(username string) error {
	newu := c.config.baseURL.JoinPath("/rest/api/2/user")
	v := newu.Query()
	v.Set("username", username)
	newu.RawQuery = v.Encode()

	resp, err := c.newRequestRoundTrip("DELETE", newu.String(), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Error deleting user user=%s", username)
	}
	return nil
}
