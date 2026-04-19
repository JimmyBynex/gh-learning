package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client wraps an *http.Client to provide REST and GraphQL helpers for GitHub APIs
type Client struct {
	http *http.Client
}

func NewClientFormHTTP(httpClient *http.Client) *Client {
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

// HTTPError is returned when the API responds with a non-2xx status code
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// apiBaseURL returns the base API URL for a hostname
func apiBaseURL(hostname string) string {
	if hostname == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + hostname + "api/v3"
}

// REST performs a REST API request and JSON-decodes the response into data
// body 是一个接口(os.File,strings.Reader,bytes.Buffer只要实现了read方法),data是零方法接口，相当于可以接受任何类型数据
func (c *Client) REST(hostname string, method string, path string, body io.Reader, data interface{}) error {
	url := apiBaseURL(hostname) + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	//保证put和post方法，才加上这个header
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseHTTPError(resp)
	}
	if data != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(data)
	}
	return nil
}

// paresHTTPError reads the response body and constructs an HTTPError.
func parseHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	// 为什么不直接使用var Message string，然后直接json.Unmarshal呢
	// 主要是因为这个json.Unmarshal相当死板，必须对应结构体和对应字段才能转换
	var apiMsg struct {
		Message string `json:"message"`
	}
	if len(body) > 0 {
		json.Unmarshal(body, &apiMsg)
	}
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Message:    apiMsg.Message,
	}
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
