package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPClient_setsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("", "2.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotUA != "GitHub CLI 2.0.0" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "GitHub CLI 2.0.0")
	}
}

func TestNewHTTPClient_setsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("mytoken", "1.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer mytoken" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer mytoken")
	}
}

func TestNewHTTPClient_noAuthWhenTokenEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("", "1.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "" {
		t.Errorf("Authorization should be empty without token, got %q", gotAuth)
	}
}

func TestNewHTTPClient_setsAPIVersion(t *testing.T) {
	var gotVer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVer = r.Header.Get("X-GitHub-Api-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("", "1.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotVer != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", gotVer, "2022-11-28")
	}
}

func TestNewHTTPClient_existingAuthNotOverwritten(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("factorytoken", "1.0.0")
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer manualtoken")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer manualtoken" {
		t.Errorf("Authorization = %q, want manual token to win", gotAuth)
	}
}
