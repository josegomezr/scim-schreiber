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

func (c *Client) ListAllGroups(displayName string) iter.Seq2[*Group, error] {
	return func(yield func(*Group, error) bool) {
		newu := c.config.baseURL.JoinPath("/rest/api/2/groups/picker")
		v := newu.Query()
		if displayName != "" {
			v.Set("query", displayName)
		}

		v.Set("maxResults", "1000")
		newu.RawQuery = v.Encode()

		groupResp := GroupListResponse{}
		n := 0
		for {
			v.Set("startAt", fmt.Sprintf("%d", n))
			newu.RawQuery = v.Encode()
			uri := newu.String()

			resp, err := c.newRequestRoundTrip("GET", uri, nil)
			if err != nil {
				yield(nil, err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				buf := new(bytes.Buffer)
				io.Copy(buf, resp.Body)
				yield(nil, fmt.Errorf("wrong status code: %d. Details=%s", resp.StatusCode, buf.String()))
				return
			}

			if err := json.NewDecoder(resp.Body).Decode(&groupResp); err != nil {
				yield(nil, fmt.Errorf("error deserializing json: %w", err))
			}

			for _, jiraGroup := range groupResp.Groups {
				// Of course JIRA can't be consistent, the "self" attribute is
				// reported on group details but not on group listing.
				// thankfully is easy to findout
				selfUrl := c.config.baseURL.JoinPath("/rest/api/2/group")
				qs := selfUrl.Query()
				qs.Set("groupname", jiraGroup.DisplayName)
				selfUrl.RawQuery = qs.Encode()
				jiraGroup.Self = selfUrl.String()

				if !yield(&jiraGroup, nil) {
					return
				}
				n += 1
			}

			if n >= groupResp.Total {
				break
			}
		}
	}
}

func (c *Client) GetGroup(displayName string) (*Group, error) {
	newu := c.config.baseURL.JoinPath("/rest/api/2/group")
	v := newu.Query()
	v.Set("groupname", displayName)
	newu.RawQuery = v.Encode()

	groupResp := Group{}
	resp, err := c.newRequestRoundTrip("GET", newu.String(), nil)
	if err != nil {
		return nil, err
	}

	// dummy not found
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(&groupResp); err != nil {
		return nil, err
	}

	memResp, err := c.GetGroupMembers(displayName)
	if err != nil {
		return nil, err
	}

	groupResp.Members = memResp

	return &groupResp, nil
}

func (c *Client) GetGroupMembers(displayName string) ([]User, error) {
	newu := c.config.baseURL.JoinPath("/rest/api/2/group/member")
	v := newu.Query()
	v.Set("groupname", displayName)
	users := []User{}
	n := 0
	for {
		v.Set("startAt", fmt.Sprintf("%d", n))
		newu.RawQuery = v.Encode()
		uri := newu.String()

		membersResp := MemberListResponse{}
		resp, err := c.newRequestRoundTrip("GET", uri, nil)
		if err != nil {
			return nil, err
		}

		// dummy not found
		if resp.StatusCode != http.StatusOK {
			return nil, nil
		}

		if err := json.NewDecoder(resp.Body).Decode(&membersResp); err != nil {
			return nil, err
		}
		for _, user := range membersResp.Users {
			users = append(users, user)
			n += 1
		}
		if n >= membersResp.Total {
			break
		}
	}
	return users, nil
}

// Jira doesn't support group name changes, it's been a known problem for 23
// years at the time of writing
// https://jira.atlassian.com/browse/JRASERVER-1391
func (c *Client) UpdateGroup(uuid string, group Group) (*Group, error) {
	return nil, fmt.Errorf("No updates for you. Come back when JRASERVER-1391 is solved")
}

// TODO(josegomezr): skip adding if the requested user exists in the members
func (c *Client) AddUserToGroup(userName string, displayName string) error {
	newu := c.config.baseURL.JoinPath("/rest/api/2/group/user")
	// This is one of the most cursed API designs I've ever seen...
	// "groupname" (group identifier) on querystring
	// "user" (username) on JSON Body
	v := newu.Query()
	v.Set("groupname", displayName)
	newu.RawQuery = v.Encode()

	memPayload := AddMemberRequest{
		UserName: userName,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(memPayload); err != nil {
		return err
	}

	req, err := c.newRequest("POST", newu.String(), buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-type", "application/json")

	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return fmt.Errorf("Error adding user=%s to group=%s. details=%s", userName, displayName, buf.String())
	}

	// The response when success is the same as "GetGroup"
	return nil
}

func (c *Client) RemoveUserFromGroup(userName string, displayName string) error {
	newu := c.config.baseURL.JoinPath("/rest/api/2/group/user")
	// But on this one, everything is on querystring... jeez
	v := newu.Query()
	v.Set("username", userName)
	v.Set("groupname", displayName)
	newu.RawQuery = v.Encode()

	resp, err := c.newRequestRoundTrip("DELETE", newu.String(), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return fmt.Errorf("Error removing user=%s, from group=%s. details=%s", userName, displayName, buf.String())
	}

	return nil
}

var GroupAlreadyExists error = errors.New("Group with this slug already exists.")

func (c *Client) CreateGroup(group Group) (*Group, error) {
	newGroup := group // Shallow copy
	newu := c.config.baseURL.JoinPath("/rest/api/2/group")
	for _, err := range c.ListAllGroups(group.DisplayName) {
		if err != nil {
			return nil, err
		}
		return nil, GroupAlreadyExists
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(newGroup); err != nil {
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
		return nil, fmt.Errorf("Could not create group. Details=%s", buf.String())
	}

	groupResp := Group{}
	if err := json.NewDecoder(resp.Body).Decode(&groupResp); err != nil {
		return nil, err
	}
	return &groupResp, nil
}

func (c *Client) DeleteGroup(displayName string) error {
	newu := c.config.baseURL.JoinPath("/rest/api/2/group")
	v := newu.Query()
	v.Set("groupname", displayName)
	newu.RawQuery = v.Encode()

	resp, err := c.newRequestRoundTrip("DELETE", newu.String(), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return fmt.Errorf("Error deleting group group=%s. details=%s", displayName, buf.String())
	}
	return nil
}
