package sitesapiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

const (
	defaultURL      = "https://sites.github.net"
	defaultUsername = "x"
	clientUserAgent = "go-sitesapi-client"
)

type StatusError struct {
	Code int
	Err  error
}

// satisfy the error interface.
func (se StatusError) Error() string {
	return se.Err.Error()
}

// Config will define connection properties for sites-api
type Config struct {
	BaseURL   string
	Username  string
	Password  string
	UserAgent string
}

// Client is a basic simple http client for interacting with the sites-api
type Client struct {
	URL        *url.URL
	Config     *Config
	httpClient *http.Client
}

// NewClient will construct and return a client.
func NewClient(httpClient *http.Client, config *Config) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	if config.UserAgent == "" {
		config.UserAgent = clientUserAgent
	}

	if config.Username == "" {
		config.Username = defaultUsername
	}

	if config.BaseURL == "" {
		config.BaseURL = defaultURL
	}

	u, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, err
	}

	c := &Client{
		URL:        u,
		Config:     config,
		httpClient: httpClient,
	}

	return c, nil
}

// NewRequest is used by all other functions to construct the request
// this is very generic and can be used as sites-api expands
func (c *Client) NewRequest(method, path string, params map[string]string, body interface{}) (*http.Request, error) {
	rel := &url.URL{Path: path}
	u := c.URL.ResolveReference(rel)

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if params != nil {
		q := req.URL.Query()
		for param, value := range params {
			q.Add(param, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.Config.UserAgent)
	req.SetBasicAuth(c.Config.Username, c.Config.Password)

	return req, nil
}

// do will submit the request and decode it when a buffer is provided
func (c *Client) do(req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if v != nil {
		if err = json.NewDecoder(resp.Body).Decode(v); err != nil {
			return nil, err
		}
	}

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, StatusError{
			Code: resp.StatusCode,
			Err:  fmt.Errorf("%d response from upstream: %s", resp.StatusCode, body),
		}
	}

	return resp, err
}
