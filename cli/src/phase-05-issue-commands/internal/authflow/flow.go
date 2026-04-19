// Package authflow implements the GitHub OAuth Device Authorization Flow.
package authflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

const (
	clientID = "178c6fc778ccc68e1d6a"
	scope    = "repo,read:org,gist"
)

// DeviceFlowResult holds the result of a successful OAuth Device Flow.
type DeviceFlowResult struct {
	Token    string
	Username string
}

// deviceCodeResponse is the JSON response from POST /login/device/code.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// tokenResponse is the JSON response from POST /login/oauth/access_token.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Interval    int    `json:"interval"`
}

// userResponse is the JSON response from GET /user.
type userResponse struct {
	Login string `json:"login"`
}

// DeviceFlow executes the GitHub OAuth Device Authorization Flow.
//
// Steps:
//  1. POST /login/device/code to obtain device_code and user_code.
//  2. Print the user_code and prompt the user to authorize in a browser.
//  3. Poll /login/oauth/access_token until authorized or expired.
//  4. GET /user to retrieve the authenticated username.
//
// httpClient should be a plain HTTP client with no Authorization header.
func DeviceFlow(httpClient *http.Client, hostname string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	return deviceFlow(httpClient, "https://"+hostname, "https://api.github.com", ios)
}

// deviceFlow is the internal implementation that accepts base URLs so tests can
// inject a local httptest.Server instead of talking to github.com.
func deviceFlow(httpClient *http.Client, ghBaseURL, apiBaseURL string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	// Step 1: Request device and user codes.
	dcResp, err := requestDeviceCode(httpClient, ghBaseURL)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}

	// Step 2: Show user code and wait for Enter.
	fmt.Fprintf(ios.Out, "! First copy your one-time code: %s\n", dcResp.UserCode)
	fmt.Fprintf(ios.Out, "Press Enter to open GitHub in your browser... ")
	_, _ = bufio.NewReader(ios.In).ReadString('\n') // ignore error; non-TTY stdin returns immediately
	fmt.Fprintf(ios.Out, "\nOpening %s\n", dcResp.VerificationURI)

	// Step 3: Poll for the access token.
	intervalSecs := dcResp.Interval
	if intervalSecs <= 0 {
		intervalSecs = 5
	}

	token, err := pollForToken(httpClient, ghBaseURL, dcResp.DeviceCode, intervalSecs)
	if err != nil {
		return nil, err
	}

	// Step 4: Fetch username.
	username, err := FetchUsername(httpClient, apiBaseURL, token)
	if err != nil {
		return nil, fmt.Errorf("fetching username: %w", err)
	}

	fmt.Fprintf(ios.Out, "Logged in as %s\n", username)
	return &DeviceFlowResult{Token: token, Username: username}, nil
}

// requestDeviceCode posts to /login/device/code and returns the parsed response.
func requestDeviceCode(httpClient *http.Client, ghBaseURL string) (*deviceCodeResponse, error) {
	endpoint := ghBaseURL + "/login/device/code"
	body := url.Values{
		"client_id": {clientID},
		"scope":     {scope},
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var dc deviceCodeResponse
	if err := json.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("decoding device code response: %w", err)
	}
	return &dc, nil
}

// pollForToken repeatedly polls the token endpoint until success or a terminal error.
func pollForToken(httpClient *http.Client, ghBaseURL, deviceCode string, intervalSecs int) (string, error) {
	endpoint := ghBaseURL + "/login/oauth/access_token"
	body := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	interval := time.Duration(intervalSecs) * time.Second

	for {
		req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return "", readErr
		}

		var tr tokenResponse
		if err := json.Unmarshal(data, &tr); err != nil {
			return "", fmt.Errorf("decoding token response: %w", err)
		}

		switch tr.Error {
		case "":
			if tr.AccessToken != "" {
				return tr.AccessToken, nil
			}
			return "", fmt.Errorf("no access token in response")
		case "authorization_pending":
			// Continue polling after the interval.
		case "slow_down":
			if tr.Interval > 0 {
				interval = time.Duration(tr.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return "", fmt.Errorf("device code expired; please try again")
		default:
			return "", fmt.Errorf("OAuth error: %s", tr.Error)
		}

		time.Sleep(interval)
	}
}

// FetchUsername calls the /user API endpoint and returns the login name.
// apiBaseURL is e.g. "https://api.github.com" or "https://ghes.example.com/api/v3".
func FetchUsername(httpClient *http.Client, apiBaseURL, token string) (string, error) {
	endpoint := apiBaseURL + "/user"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/user returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var u userResponse
	if err := json.Unmarshal(data, &u); err != nil {
		return "", fmt.Errorf("decoding user response: %w", err)
	}
	if u.Login == "" {
		return "", fmt.Errorf("empty login in /user response")
	}
	return u.Login, nil
}
