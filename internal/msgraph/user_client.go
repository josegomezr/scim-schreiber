package msgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"
)

func (c *Client) GetUser(uuid string) (*User, error) {
	newu := c.config.baseURL.JoinPath("/users/", uuid)
	v := newu.Query()
	v.Set("$select", msGraphUserFields)
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

func (c *Client) ListAllUsers(principal string) iter.Seq2[*User, error] {
	return func(yield func(*User, error) bool) {
		newu := c.config.baseURL.JoinPath("/users/")
		v := newu.Query()
		v.Set("$top", "100")
		v.Set("$count", "true")
		v.Set("$select", msGraphUserFields)
		if principal != "" {
			// MSGraph uses single quote for filter, just amazing...
			filterExpr := fmt.Sprintf(`userPrincipalName eq '%s@%s'`, principal, c.config.TenantDomain)
			v.Set("$filter", filterExpr)
		}

		newu.RawQuery = v.Encode()

		userResp := UserListResponse{}
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

			json.NewDecoder(resp.Body).Decode(&userResp)
			for _, msUser := range userResp.Users {
				if !yield(&msUser, nil) {
					return
				}
			}

			if userResp.NextLink == "" || uri == userResp.NextLink {
				break
			}
			uri = userResp.NextLink
		}
	}
}

func (c *Client) UpdateUser(uuid string, user User) (*User, error) {
	newu := c.config.baseURL.JoinPath("/users/", uuid)

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(user); err != nil {
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
		return nil, fmt.Errorf("Error patching user id=%s, status=%d. details=%s", uuid, resp.StatusCode, buf.String())
	}

	return c.GetUser(uuid)
}

func (c *Client) CreateUser(user User) (*User, error) {
	newUser := user // Shallow copy
	newUser.UserPrincipalName = fmt.Sprintf("%s@%s", user.UserPrincipalName, c.config.TenantDomain)

	newu := c.config.baseURL.JoinPath("/users/")

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
	newUser.Id = userResp.Id
	return &newUser, nil
}

func (c *Client) DeleteUser(uuid string) error {
	// Delete from recycle bin, yes, as you read it...
	newu := c.config.baseURL.JoinPath("/directory/deletedItems/", uuid)
	c.newRequestRoundTrip("DELETE", newu.String(), nil)

	newu = c.config.baseURL.JoinPath("/users/", uuid)
	currentUser, err := c.GetUser(uuid)
	if err != nil {
		return fmt.Errorf("Could not find User. Details=%s", err)
	}

	// If not found, early exit, not much we can do, the recycle bin was already
	// triggered in the request above.
	if currentUser == nil {
		return nil
	}

	// So, this is ugly, but simplicity wins. MS Graph API is eventually
	// consistent, and will only allow changes to the OnPremisesImmutableId if
	// and only if the UserPrincipalName is not "federated" (meaning is something
	// else but a *.onmicrosoft.com domain)
	//
	// Without this dance deletions will land on the recycle bin (yes, that
	// recycle bin).
	//
	// We'll mark the user as inative so it can't really use anything, and then
	// change the principal name.
	//
	// Then we check 7 times with a 5 seconds wait in between if the principal
	// name change was propagated. if until that it hasn't changed, then we'll
	// roll it and go with the flow all the way to the actual deletion.
	//
	// The consequence of this is having users as "to-be-deleted" in the
	// directory, they're easy enough to spot and cleanup.
	//
	// Retries will cleanup the recycle bin in case the principal name change
	// failed.
	if !strings.HasPrefix("to-be-deleted-", currentUser.UserPrincipalName) {
		newPrincipalName := fmt.Sprintf("to-be-deleted-%s@%s", uuid, c.config.OnMicrosoftDomain)
		userModel := User{
			UserPrincipalName: newPrincipalName,
			AccountEnabled:    false,
		}

		if _, err := c.UpdateUser(uuid, userModel); err != nil {
			return fmt.Errorf("Could not change UserPrincipalName. Details=%s", err)
		}

		times := 0
		for times < 7 {
			time.Sleep(5 * time.Second)
			times += 1
			newuser, err := c.GetUser(uuid)
			if err != nil || newuser.UserPrincipalName == newPrincipalName {
				break
			}
		}
	}

	if !strings.HasPrefix(currentUser.OnPremisesImmutableId, "to-be-deleted-") {
		newImmutableId := fmt.Sprintf("%s-%s", "to-be-deleted", uuid)
		userModel := User{
			OnPremisesImmutableId: newImmutableId,
			AccountEnabled:        false,
		}
		c.UpdateUser(uuid, userModel)
	}

	resp, err := c.newRequestRoundTrip("DELETE", newu.String(), nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Error deleting user user=%s", uuid)
	}
	return nil
}
