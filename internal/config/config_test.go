package config

import "testing"

func TestConfigValidateProductionAcceptsJWTSecret(t *testing.T) {
	cfg := Config{Env: "production", JWTSecret: "jwt-secret", AuthDownloadMode: "proxy"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigValidateProductionRequiresAPIKey(t *testing.T) {
	cfg := Config{Env: "production", APIKey: "", AuthDownloadMode: "proxy"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for production without API_KEY")
	}
}

func TestConfigValidateProductionAcceptsAPIKeys(t *testing.T) {
	t.Setenv("API_KEYS", "key-a,key-b")
	cfg := Config{Env: "production", AuthDownloadMode: "proxy"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.APIKeys()) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(cfg.APIKeys()))
	}
}

func TestConfigValidateDevelopmentAllowsEmptyAPIKey(t *testing.T) {
	cfg := Config{Env: "development", APIKey: "", AuthDownloadMode: "proxy"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigValidateProductionWithAPIKey(t *testing.T) {
	cfg := Config{Env: "production", APIKey: "secret", AuthDownloadMode: "proxy"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigValidateAuthDownloadMode(t *testing.T) {
	cfg := Config{AuthDownloadMode: "invalid"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid AUTH_DOWNLOAD_MODE error")
	}
	cfg.AuthDownloadMode = "redirect"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
