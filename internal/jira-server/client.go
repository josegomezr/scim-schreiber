package jira

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

type Config struct {
	Token                  string
	ServerUrl              string
	IncludeMembersInGroups bool
	baseURL                *url.URL
}

type Client struct {
	config    *Config
	authToken string
}

type ConfiguratorFn func(*Config)

func NewClient(fns ...ConfiguratorFn) (*Client, error) {
	cfg := &Config{}
	for _, fn := range fns {
		fn(cfg)
	}

	u, err := url.Parse(cfg.ServerUrl)
	if err != nil {
		return nil, err
	}

	cfg.baseURL = u
	return &Client{
		config:    cfg,
		authToken: cfg.Token,
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
	req, err := c.newRawRequest(method, uri, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)

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
	slog.Info("Jira request", "method", req.Method, "url", req.URL.String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	slog.Debug("Jira response", "status", resp.StatusCode)
	return resp, err
}
