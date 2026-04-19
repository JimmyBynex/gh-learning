package factory_test

import (
	"testing"

	"github.com/learngh/gh-impl/internal/factory"
)

func TestNew_nonNil(t *testing.T) {
	f := factory.New("1.0.0")
	if f == nil {
		t.Fatal("expected non-nil Factory")
	}
}

func TestNew_appVersion(t *testing.T) {
	f := factory.New("2.40.0")
	if f.AppVersion != "2.40.0" {
		t.Errorf("AppVersion = %q, want %q", f.AppVersion, "2.40.0")
	}
}

func TestNew_executableName(t *testing.T) {
	f := factory.New("1.0.0")
	if f.ExecutableName != "gh" {
		t.Errorf("ExecutableName = %q, want %q", f.ExecutableName, "gh")
	}
}

func TestNew_ioStreams(t *testing.T) {
	f := factory.New("1.0.0")
	if f.IOStreams == nil {
		t.Error("expected non-nil IOStreams")
	}
}

func TestNew_config_returnsStub(t *testing.T) {
	f := factory.New("1.0.0")
	if f.Config == nil {
		t.Fatal("Config getter is nil")
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("Config() returned nil config")
	}
}

func TestNew_config_stub_methods_return_errors(t *testing.T) {
	f := factory.New("1.0.0")
	cfg, _ := f.Config()

	if _, err := cfg.Get("github.com", "token"); err == nil {
		t.Error("stub Config.Get should return error")
	}
	if err := cfg.Set("github.com", "token", "val"); err == nil {
		t.Error("stub Config.Set should return error")
	}
	if err := cfg.Write(); err == nil {
		t.Error("stub Config.Write should return error")
	}
}

func TestNew_httpClient_returnsNonNil(t *testing.T) {
	f := factory.New("1.0.0")
	if f.HttpClient == nil {
		t.Fatal("HttpClient getter is nil")
	}
	client, err := f.HttpClient()
	if err != nil {
		t.Fatalf("HttpClient() returned unexpected error: %v", err)
	}
	if client == nil {
		t.Error("HttpClient() returned nil client")
	}
}

func TestNew_gitClient_nil(t *testing.T) {
	f := factory.New("1.0.0")
	if f.GitClient != nil {
		t.Error("expected GitClient to be nil in Phase 1")
	}
}
