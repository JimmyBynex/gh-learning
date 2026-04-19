package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client wraps an *http.Client to provide REST and GraphQL helpers for GitHub APIs.
type Client struct {
	http *http.Client
}

// NewClientFromHTTP creates an API Client from an existing *http.Client.
func NewClientFromHTTP(httpClient *http.Client) *Client {
	return &Client{http: httpClient}
}

// GraphQLErrorItem is a single error item in a GraphQL response.
type GraphQLErrorItem struct {
	Message   string
	Locations []struct{ Line, Column int }
	Path      []string
}

// GraphQLError is returned when the GraphQL response contains errors.
type GraphQLError struct {
	Message string
	Errors  []GraphQLErrorItem
}

func (e GraphQLError) Error() string {
	return e.Message
}

// HTTPError is returned when the API responds with a non-2xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// apiBaseURL returns the base API URL for a hostname.
func apiBaseURL(hostname string) string {
	if hostname == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + hostname + "/api/v3"
}

// REST performs a REST API request and JSON-decodes the response into data.
// path may include or omit a leading slash.
func (c *Client) REST(hostname, method, path string, body io.Reader, data interface{}) error {
	url := apiBaseURL(hostname) + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseHTTPError(resp)
	}

	if data != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(data)
	}
	return nil
}

// parseHTTPError reads the response body and constructs an HTTPError.
func parseHTTPError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var apiMsg struct {
		Message string `json:"message"`
	}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &apiMsg)
	}
	return HTTPError{StatusCode: resp.StatusCode, Message: apiMsg.Message}
}

// GraphQL performs a GraphQL query against the API and decodes the response data into data.
func (c *Client) GraphQL(hostname, query string, variables map[string]interface{}, data interface{}) error {
	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables,omitempty"`
	}{Query: query, Variables: variables}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := apiBaseURL(hostname) + "/graphql"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseHTTPError(resp)
	}

	var gqlResp struct {
		Data   json.RawMessage    `json:"data"`
		Errors []GraphQLErrorItem `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return err
	}

	if len(gqlResp.Errors) > 0 {
		return GraphQLError{
			Message: gqlResp.Errors[0].Message,
			Errors:  gqlResp.Errors,
		}
	}

	if data != nil && gqlResp.Data != nil {
		return json.Unmarshal(gqlResp.Data, data)
	}
	return nil
}
