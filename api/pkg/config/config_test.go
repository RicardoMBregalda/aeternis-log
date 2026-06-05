package config

import (
	"testing"
	"time"
)

func TestValidateAuth(t *testing.T) {
	c, err := LoadConfig("")
	if err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}

	// Enabling auth without any keys is a misconfiguration (would lock everyone
	// out), so it must fail validation.
	c.Auth.Enabled = true
	c.Auth.APIKeys = nil
	if err := c.Validate(); err == nil {
		t.Error("expected error when auth is enabled without api_keys")
	}

	c.Auth.APIKeys = []string{"a-key"}
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error with a configured key: %v", err)
	}

	c.Auth.HeaderName = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error when auth header_name is empty")
	}
}

func TestValidateRateLimit(t *testing.T) {
	c, err := LoadConfig("")
	if err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}

	c.RateLimit.Enabled = true
	c.RateLimit.MaxRequests = 0
	if err := c.Validate(); err == nil {
		t.Error("expected error when max_requests < 1")
	}

	c.RateLimit.MaxRequests = 10
	c.RateLimit.Window = 0
	if err := c.Validate(); err == nil {
		t.Error("expected error when window is not positive")
	}

	c.RateLimit.Window = time.Minute
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error with valid rate limit: %v", err)
	}
}

func TestValidateFabric(t *testing.T) {
	// docker-exec transport requires peer_container.
	c, err := LoadConfig("")
	if err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	c.Fabric.SyncEnabled = true
	c.Fabric.Transport = "docker-exec"
	c.Fabric.PeerContainer = ""
	if err := c.Validate(); err == nil {
		t.Error("docker-exec: expected error without peer_container")
	}

	// gateway transport requires the identity/cert settings.
	c2, err := LoadConfig("")
	if err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	c2.Fabric.SyncEnabled = true
	c2.Fabric.Transport = "gateway"
	c2.Fabric.IdentityCertFile = ""
	if err := c2.Validate(); err == nil {
		t.Error("gateway: expected error without identity_cert_file")
	}
}

func TestSplitAndTrim(t *testing.T) {
	got := splitAndTrim(" a, b ,, c ", ",")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(splitAndTrim("", ",")) != 0 {
		t.Error("empty input should yield no items")
	}
}
