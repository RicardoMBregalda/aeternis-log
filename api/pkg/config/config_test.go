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

func TestChannelForTenant(t *testing.T) {
	fc := &FabricConfig{
		Channel: "logchannel",
		TenantChannels: map[string]string{
			"acme":   "acme-channel",
			"globex": "globex-channel",
		},
	}
	// Mapped tenants resolve to their dedicated channel.
	if got := fc.ChannelForTenant("acme"); got != "acme-channel" {
		t.Errorf("acme: got %q, want acme-channel", got)
	}
	// Unmapped tenants fall back to the default channel.
	if got := fc.ChannelForTenant("default"); got != "logchannel" {
		t.Errorf("default: got %q, want logchannel", got)
	}
	if got := fc.ChannelForTenant("unknown"); got != "logchannel" {
		t.Errorf("unknown tenant should fall back to default, got %q", got)
	}
	// With no mapping at all, everything resolves to the default channel.
	bare := &FabricConfig{Channel: "logchannel"}
	if got := bare.ChannelForTenant("acme"); got != "logchannel" {
		t.Errorf("no mapping: got %q, want logchannel", got)
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
