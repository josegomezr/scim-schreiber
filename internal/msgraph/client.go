package msgraph

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

const msGraphUserFields = "id,givenName,surname,displayName,mailNickname,userPrincipalName,accountEnabled,onPremisesImmutableId"
const msGraphMembershipFields = "id,displayName,userPrincipalName"
const msGraphGroupFields = "id,displayName,createdDateTime"

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

func (c *Client) newRequestRoundTrip(method, uri string, body io.Reader) (*http.Response, error) {
	req, err := c.newRequest(method, uri, body)
	if err != nil {
		return nil, err
	}
	return c.doRequest(req)
}
func (c *Client) newRequest(method, uri string, body io.Reader) (*http.Request, error) {
	{
		c.mutex.Lock()
		defer c.mutex.Unlock()
		if c.authToken == "" || time.Now().After(c.authExpires) {
			err := c.NegotiateAccessToken()
			if err != nil {
				return nil, err
			}
		}
	}

	req, err := c.newRawRequest(method, uri, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("ConsistencyLevel", "eventual")

	return req, nil
}

func (c *Client) newRawRequest(method, uri string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (c *Client) doRequest(req *http.Request) (*http.Response, error) {
	slog.Info("MS-Graph request", "method", req.Method, "url", req.URL.String())
	resp, err := http.DefaultClient.Do(req)
	slog.Debug("MS-Graph response", "status", resp.StatusCode)
	return resp, err
}

func (c *Client) NegotiateAccessToken() error {
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
		return fmt.Errorf("Could not authenticate with the given credentials.")
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
