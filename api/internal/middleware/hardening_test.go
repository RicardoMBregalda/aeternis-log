package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMatchAPIKeyHashed covers F05: a configured key may be a "sha256:<hex>"
// hash (so the raw key is never stored) and plaintext keys still work.
func TestMatchAPIKeyHashed(t *testing.T) {
	sum := sha256.Sum256([]byte("secret-key"))
	hashed := "sha256:" + hex.EncodeToString(sum[:])
	m := map[string]string{
		hashed:      "acme",
		"plain-key": "globex",
	}

	if tenant, ok := matchAPIKey("secret-key", m); !ok || tenant != "acme" {
		t.Errorf("hashed key: got (%q, %v), want (acme, true)", tenant, ok)
	}
	if tenant, ok := matchAPIKey("plain-key", m); !ok || tenant != "globex" {
		t.Errorf("plaintext key: got (%q, %v), want (globex, true)", tenant, ok)
	}
	if _, ok := matchAPIKey("nope", m); ok {
		t.Error("an unknown key must not match")
	}
}

// TestCORS covers F08: "*" must never be paired with Allow-Credentials, an
// explicit allowlisted origin is reflected with credentials, and a non-listed
// origin gets no Allow-Origin header.
func TestCORS(t *testing.T) {
	tests := []struct {
		name            string
		allowed         []string
		origin          string
		wantAllowOrigin string
		wantCreds       string // "" means the header must be absent
	}{
		{"wildcard reflects star without credentials", []string{"*"}, "https://evil.example", "*", ""},
		{"explicit origin allowed with credentials", []string{"https://app.example"}, "https://app.example", "https://app.example", "true"},
		{"non-listed origin denied", []string{"https://app.example"}, "https://other.example", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(CORS(tt.allowed))
			r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Origin", tt.origin)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantAllowOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantAllowOrigin)
			}
			if got := w.Header().Get("Access-Control-Allow-Credentials"); got != tt.wantCreds {
				t.Errorf("Allow-Credentials = %q, want %q", got, tt.wantCreds)
			}
			// Invariant: "*" and credentials must never coexist.
			if w.Header().Get("Access-Control-Allow-Origin") == "*" &&
				w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Error("invalid CORS: wildcard origin paired with credentials")
			}
		})
	}
}

// TestValidateDomain covers F07's domain guard.
func TestValidateDomain(t *testing.T) {
	r := gin.New()
	r.Use(ValidateDomain())
	r.GET("/api/v1/:domain/records", func(c *gin.Context) { c.Status(http.StatusOK) })

	tests := []struct {
		domain     string
		wantStatus int
	}{
		{"contracts", http.StatusOK},
		{"logs", http.StatusOK},
		{"a-b-9", http.StatusOK},
		{"BadCase", http.StatusBadRequest},
		{"has_underscore", http.StatusBadRequest},
		{"-leadinghyphen", http.StatusBadRequest},
		{"dot.domain", http.StatusBadRequest},
		{strings.Repeat("a", 64), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/"+tt.domain+"/records", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("domain %q: status = %d, want %d", tt.domain, w.Code, tt.wantStatus)
			}
		})
	}
}

// TestMaxBodyBytes covers F07's body cap: a body over the limit must not be
// readable in full (the read errors), preventing memory exhaustion.
func TestMaxBodyBytes(t *testing.T) {
	const limit = 16

	r := gin.New()
	r.Use(MaxBodyBytes(limit))
	r.POST("/x", func(c *gin.Context) {
		if _, err := c.GetRawData(); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"within limit", "small", http.StatusOK},
		{"over limit", strings.Repeat("x", limit+1), http.StatusRequestEntityTooLarge},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
