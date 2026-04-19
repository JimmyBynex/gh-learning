package authflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

// mockServer creates an httptest.Server that mocks GitHub's OAuth and API endpoints.
// pendingCount controls how many "authorization_pending" responses are returned
// before a successful token is issued.
func mockServer(t *testing.T, pendingCount int) *httptest.Server {
	t.Helper()
	attempts := 0

	mux := http.NewServeMux()

	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(deviceCodeResponse{
			DeviceCode:      "dev_code_abc",
			UserCode:        "TEST-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        1, // 1 second minimum so tests finish quickly
		}); err != nil {
			t.Errorf("encode device code: %v", err)
		}
	})

	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if attempts < pendingCount {
			attempts++
			_ = json.NewEncoder(w).Encode(tokenResponse{Error: "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "ghp_mock_token",
			TokenType:   "bearer",
		})
	})

	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userResponse{Login: "mockuser"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDeviceFlow_EndToEnd(t *testing.T) {
	srv := mockServer(t, 0)
	ios, inBuf, _, _ := iostreams.Test()
	inBuf.WriteString("\n") // simulate user pressing Enter

	result, err := deviceFlow(srv.Client(), srv.URL, srv.URL, ios)
	if err != nil {
		t.Fatalf("deviceFlow: %v", err)
	}
	if result.Token != "ghp_mock_token" {
		t.Errorf("Token = %q, want ghp_mock_token", result.Token)
	}
	if result.Username != "mockuser" {
		t.Errorf("Username = %q, want mockuser", result.Username)
	}
}

func TestDeviceFlow_AuthorizationPending(t *testing.T) {
	srv := mockServer(t, 2) // 2 pending before success
	ios, inBuf, _, _ := iostreams.Test()
	inBuf.WriteString("\n")

	result, err := deviceFlow(srv.Client(), srv.URL, srv.URL, ios)
	if err != nil {
		t.Fatalf("deviceFlow with pending: %v", err)
	}
	if result.Token != "ghp_mock_token" {
		t.Errorf("Token = %q, want ghp_mock_token", result.Token)
	}
}

func TestDeviceFlow_ExpiredToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deviceCodeResponse{
			DeviceCode: "code", UserCode: "CODE-0000",
			VerificationURI: "https://github.com/login/device",
			Interval:        1,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "expired_token"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ios, inBuf, _, _ := iostreams.Test()
	inBuf.WriteString("\n")

	_, err := deviceFlow(srv.Client(), srv.URL, srv.URL, ios)
	if err == nil {
		t.Fatal("expected error for expired_token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to contain 'expired'", err.Error())
	}
}

func TestRequestDeviceCode_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := requestDeviceCode(srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestFetchUsername_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := FetchUsername(srv.Client(), srv.URL, "bad_token")
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}

func TestPollForToken_SlowDown(t *testing.T) {
	attempts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deviceCodeResponse{
			DeviceCode: "code", UserCode: "CODE-0001",
			VerificationURI: "https://github.com/login/device",
			Interval:        1,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if attempts == 0 {
			attempts++
			// slow_down with a server-provided interval of 1 second so the test stays fast.
			_ = json.NewEncoder(w).Encode(tokenResponse{Error: "slow_down", Interval: 1})
			return
		}
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "ghp_slow_token", TokenType: "bearer"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userResponse{Login: "slowuser"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ios, inBuf, _, _ := iostreams.Test()
	inBuf.WriteString("\n")

	result, err := deviceFlow(srv.Client(), srv.URL, srv.URL, ios)
	if err != nil {
		t.Fatalf("deviceFlow with slow_down: %v", err)
	}
	if result.Token != "ghp_slow_token" {
		t.Errorf("Token = %q, want ghp_slow_token", result.Token)
	}
}
