package authflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"scratch/pkg/iostreams"
	"strings"
	"time"
)

// 开发者注册的clientID，以及对应权限
const (
	clientID = "178c6fc778ccc68e1d6a"
	scope    = "repo,read:org,gist"
)

type DeviceFlowResult struct {
	Token    string
	Username string
}

// deviceCodeFlowResponse holds the response for the first time's verification
// the user will see the VerificationURI and UserCode
type deviceCodeFlowResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// tokenResponse holds the second time's response while polling from server whether the user logins
// if it already logins,error==""
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Interval    int    `json:"interval"`
}

// userResponse holds the third time's response (username) according to the given token
type userResponse struct {
	Login string `json:"login"`
}

// DeviceFlow executes the github OAuth device authorization flow
// Steps:
//  1. POST /login/device/code to obtain device_code and user_code.
//  2. Print the user_code and prompt the user to authorize in a browser.
//  3. Poll /login/oauth/access_token until authorized or expired.
//  4. GET /user to retrieve the authenticated username.
func DeviceFlow(httpClient *http.Client, hostname string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	return deviceFlow(httpClient, "https://"+hostname, "https://api.github.com", ios)
}

// deviceFlow is the internal implement of DeviceFlow
func deviceFlow(httpClient *http.Client, ghBaseURL, apiBaseURL string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	//1.POST /login/device/code/code to obtain device_code and user_code
	deviceFlowRep, err := requestDeviceCode(httpClient, ghBaseURL)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	//2.Print the user_code and prompt the user to authorize in a browser
	fmt.Fprintf(ios.Out, "! First copy your one-time code: %s\n", deviceFlowRep.UserCode)
	fmt.Fprintf(ios.Out, "Press Enter to open GitHub in your browser... ")
	//相当于一个阻塞
	_, _ = bufio.NewReader(ios.In).ReadString('\n')
	fmt.Fprintf(ios.Out, "\nOpening %s\n", deviceFlowRep.VerificationURI)

	intervalSecs := deviceFlowRep.Interval
	if intervalSecs <= 0 {
		intervalSecs = 5
	}
	//3.POLL /login/oauth/access_token until authorized or expired
	token, err := pollForToken(httpClient, ghBaseURL, deviceFlowRep.DeviceCode, intervalSecs)
	if err != nil {
		//polling 内部已经处理错误了
		return nil, err
	}
	//4.GET /user
	username, err := FetchUserName(httpClient, apiBaseURL, token)
	if err != nil {
		return nil, fmt.Errorf("fetching user name: %w", err)
	}
	fmt.Fprintf(ios.Out, "logged in as %s\n", username)
	return &DeviceFlowResult{Token: token, Username: username}, nil
}

// requestDeviceCode post to /login/device/code and then returns the parse data
func requestDeviceCode(httpClient *http.Client, ghBaseURL string) (*deviceCodeFlowResponse, error) {
	//1.先是组装目标地址
	endpoint := ghBaseURL + "/login/device/code"
	//2.组装body
	body := url.Values{
		"client_id": {clientID},
		"scope":     {scope},
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	//3.组装header
	//告知服务器我的格式是client_id=xxx&&socpe=xxx，我要的接受格式是json
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	//接受返回值的第一步先看信号
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request returned HTTP %d", resp.StatusCode)
	}
	//接着读取再转换格式
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result deviceCodeFlowResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("device code request returned JSON parse error: %s", err)
	}
	return &result, nil
}

// pollForToken constantly posts to /login/oauth/access_token and eventually returns the access_token
func pollForToken(httpClient *http.Client, ghBaseURL string, deviceCode string, intervalSecs int) (string, error) {
	endPoint := ghBaseURL + "/login/oauth/access_token"
	body := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	interval := time.Duration(intervalSecs) * time.Second
	//start polling
	for {
		//要把req的创建放进来，因为req会把一开始的body读完，第二次就空了
		req, err := http.NewRequest(http.MethodPost, endPoint, strings.NewReader(body.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("polling for token failed with HTTP %w", err)
		}
		//defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("polling for token failed with HTTP %d", resp.StatusCode)
		}

		var result tokenResponse
		data, err := io.ReadAll(resp.Body)
		//这里必须手动关闭，因为在循环里面，如果用defer就会累积到最后一次才关闭
		resp.Body.Close()

		if err != nil {
			return "", err
		}
		if err = json.Unmarshal(data, &result); err != nil {
			return "", fmt.Errorf("polling for token failed with JSON parse error: %s", err)
		}

		//进来分类判断，只有“”才是可能返回access_token
		switch result.Error {
		case "":
			if result.AccessToken != "" {
				return result.AccessToken, nil
			}
			return "", fmt.Errorf("No access token in response")
		case "authorization_pending":
			//continue
		case "slow_down":
			if result.Interval > 0 {
				interval = time.Duration(result.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return "", fmt.Errorf("Token expired.Please try again")
		default:
			return "", fmt.Errorf("OAuth error: %s", result.Error)
		}
		//间隔
		time.Sleep(interval)
	}
}

// FetchUserName verifies the access_token and return the according user's name
// 这里写为外部可调用的原因是除了deviceFlow使用以外，withToken分支也会使用
func FetchUserName(httpClient *http.Client, apiBaseURL, token string) (string, error) {
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
		return "", fmt.Errorf("fetching user name failed with HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	var u userResponse
	if err := json.Unmarshal(data, &u); err != nil {
		return "", fmt.Errorf("decoding user response: %w", err)
	}
	if u.Login == "" {
		return "", fmt.Errorf("empty login in /user response")
	}
	return u.Login, nil
}
