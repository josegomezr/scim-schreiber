package msgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ClientId          string
	ClientSecret      string
	OnMicrosoftDomain string
	TenantId          string
	TenantDomain      string
	AuthURL           string
	GraphAPIBaseURL   string
	baseURL           *url.URL
}

const msGraphBase = "https://graph.microsoft.com/v1.0"

type Client struct {
	config      *Config
	authToken   string
	authExpires time.Time
	mutex       *sync.Mutex
}

type ConfiguratorFn func(*Config)

const FIELDS = "id,givenName,surname,displayName,mailNickname,userPrincipalName,accountEnabled,onPremisesImmutableId"

func NewClient(fns ...ConfiguratorFn) (*Client, error) {
	cfg := &Config{}
	for _, fn := range fns {
		fn(cfg)
	}

	if cfg.AuthURL == "" {
		cfg.AuthURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", cfg.TenantId)
	}
	if cfg.GraphAPIBaseURL == "" {
		cfg.GraphAPIBaseURL = msGraphBase
	}

	u, err := url.Parse(cfg.GraphAPIBaseURL)
	if err != nil {
		return nil, err
	}

	cfg.baseURL = u
	return &Client{
		config: cfg,
		mutex:  &sync.Mutex{},
	}, nil
}

func (c *Client) GetUser(uuid string) (*User, error) {
	newu := c.config.baseURL.JoinPath(fmt.Sprintf("/users/%s", uuid))
	v := newu.Query()
	v.Set("$select", "id,givenName,surname,displayName,mailNickname,userPrincipalName,accountEnabled,onPremisesImmutableId")
	newu.RawQuery = v.Encode()

	userResp := User{}
	resp, err := c.newRequest("GET", newu.String(), nil, nil)
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
		v.Set("$top", "20")
		v.Set("$count", "true")
		v.Set("$select", "id,givenName,surname,displayName,mailNickname,userPrincipalName,accountEnabled,onPremisesImmutableId")
		if principal != "" {
			// MSGraph uses single quote for filter, just amazing...
			filterExpr := fmt.Sprintf(`userPrincipalName eq '%s@%s'`, principal, c.config.TenantDomain)
			v.Set("$filter", filterExpr)
		}

		newu.RawQuery = v.Encode()

		userResp := UserListResponse{}
		uri := newu.String()
		for {
			// TODO(josegomezr): check status code of ms response
			resp, err := c.newRequest("GET", uri, nil, nil)
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
	newu := c.config.baseURL.JoinPath(fmt.Sprintf("/users/%s", uuid))

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(user); err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	headers["Content-type"] = "application/json"

	resp, err := c.newRequest("PATCH", newu.String(), headers, buf)
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

	headers := make(map[string]string)
	headers["Content-type"] = "application/json"

	resp, err := c.newRequest("POST", newu.String(), headers, buf)
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
	newu := c.config.baseURL.JoinPath(fmt.Sprintf("/users/%s", uuid))

	currentUser, err := c.GetUser(uuid)
	if err != nil {
		return fmt.Errorf("Could not find User. Details=%s", err)
	}

	currentPrincipal := currentUser.UserPrincipalName

	if !strings.HasPrefix("to-be-deleted-", currentPrincipal) {
		newPrincipalName := fmt.Sprintf("to-be-deleted-%s@%s", uuid, c.config.OnMicrosoftDomain)
		userModel := User{
			UserPrincipalName: newPrincipalName,
		}

		if _, err := c.UpdateUser(uuid, userModel); err != nil {
			return fmt.Errorf("Could not change UserPrincipalName. Details=%s", err)
		}

		// So, this is ugly, but simplicity wins. MS Graph API is eventually
		// consistent, and will only allow changes to the OnPremisesImmutableId if
		// and only if the UserPrincipalName is not "federated" (meaning is something
		// else but a *.onmicrosoft.com domain)
		//
		// Without this dance deletions can take many hours (if not days) to settle.
		//
		// We'll check 7 times with a 5 seconds wait in between if the principal
		// name change was propagated., if until that it hasn't changed, then we'll
		// roll it and go with the flow.
		//
		// The consequence of this is having users as "to-be-deleted" in the
		// directory, they're easy enough to spot and cleanup.

		times := 0
		for times < 7 {
			times += 1
			newuser, err := c.GetUser(uuid)
			if err != nil || currentPrincipal == newuser.UserPrincipalName {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}

	if !strings.HasPrefix(currentUser.OnPremisesImmutableId, "to-be-deleted-") {
		newImmutableId := fmt.Sprintf("%s-%s", "to-be-deleted", uuid)
		userModel := User{
			OnPremisesImmutableId: newImmutableId,
		}

		if _, err := c.UpdateUser(uuid, userModel); err != nil {
			return fmt.Errorf("Could not change OnPremisesImmutableId. Details=%s", err)
		}
	}

	resp, err := c.newRequest("DELETE", newu.String(), nil, nil)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Error deleting user user=%s", uuid)
	}
	return nil
}

func (c *Client) negotiateAccessToken() error {
	h := url.Values{}
	h.Set("client_id", c.config.ClientId)
	h.Set("client_secret", c.config.ClientSecret)
	h.Set("scope", "https://graph.microsoft.com/.default")
	h.Set("grant_type", "client_credentials")

	slog.Info("MS-Graph Auth", "client_id", c.config.ClientId)
	resp, err := http.PostForm(c.config.AuthURL, h)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Wrong auth response")
	}
	ar := AuthTokenResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return err
	}
	c.authToken = ar.AccessToken
	c.authExpires = time.Now().Add(time.Second * time.Duration(ar.ExpiresIn))

	slog.Info("MS-Graph Authorized", "expires_at", c.authExpires)

	return nil
}

func (c *Client) newRequest(method, uri string, headers map[string]string, body io.Reader) (*http.Response, error) {
	if headers == nil {
		headers = make(map[string]string)
	}

	{
		c.mutex.Lock()
		defer c.mutex.Unlock()
		if c.authToken == "" || time.Now().After(c.authExpires) {
			err := c.negotiateAccessToken()
			if err != nil {
				return nil, err
			}
		}
	}

	headers["Authorization"] = "Bearer " + c.authToken
	headers["ConsistencyLevel"] = "eventual"

	return c.newRawRequest(method, uri, headers, body)
}

func (c *Client) newRawRequest(method, uri string, headers map[string]string, body io.Reader) (*http.Response, error) {
	slog.Info("MS-Graph request", "method", method, "uri", uri)
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return nil, err
	}

	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	slog.Debug("MS-Graph response", "status", resp.StatusCode)

	return resp, err
}
