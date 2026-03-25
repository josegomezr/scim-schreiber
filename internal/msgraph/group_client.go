package msgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
)

func (c *Client) ListAllGroups(displayName string) iter.Seq2[*Group, error] {
	return func(yield func(*Group, error) bool) {
		newu := c.config.baseURL.JoinPath("/groups/")
		v := newu.Query()
		v.Set("$top", "100")
		v.Set("$count", "true")
		v.Set("$select", msGraphGroupFields)
		if displayName != "" {
			// MSGraph uses single quote for filter, just amazing...
			filterExpr := fmt.Sprintf(`displayName eq '%s'`, displayName)
			v.Set("$filter", filterExpr)
		}

		newu.RawQuery = v.Encode()

		groupResp := GroupListResponse{}
		uri := newu.String()
		for {
			resp, err := c.newRequestRoundTrip("GET", uri, nil)
			if err != nil {
				yield(nil, err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				yield(nil, fmt.Errorf("wrong status code: %d", resp.StatusCode))
				return
			}

			json.NewDecoder(resp.Body).Decode(&groupResp)
			for _, msgroup := range groupResp.Groups {
				if !yield(&msgroup, nil) {
					return
				}
			}

			if groupResp.NextLink == "" || uri == groupResp.NextLink {
				break
			}
			uri = groupResp.NextLink
		}
	}
}

func (c *Client) GetGroup(uuid string) (*Group, error) {
	newu := c.config.baseURL.JoinPath("/groups/", uuid)
	v := newu.Query()
	v.Set("$select", msGraphGroupFields)
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
	return &groupResp, nil
}

func (c *Client) UpdateGroup(uuid string, group Group) (*Group, error) {
	newu := c.config.baseURL.JoinPath("/groups/", uuid)

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(group); err != nil {
		return nil, err
	}

	req, err := c.newRequest("PATCH", newu.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusNoContent {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return nil, fmt.Errorf("Error patching group id=%s, status=%d. details=%s", uuid, resp.StatusCode, buf.String())
	}

	return c.GetGroup(uuid)
}

func (c *Client) CreateGroup(group Group) (*Group, error) {
	newGroup := group // Shallow copy
	newu := c.config.baseURL.JoinPath("/groups/")

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
	newGroup.Id = groupResp.Id
	return &newGroup, nil
}

func (c *Client) DeleteGroup(uuid string) error {
	newu := c.config.baseURL.JoinPath("/groups/", uuid)
	resp, err := c.newRequestRoundTrip("DELETE", newu.String(), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Error deleting group group=%s", uuid)
	}
	return nil
}
