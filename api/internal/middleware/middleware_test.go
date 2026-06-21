package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAPIKeyAuth(t *testing.T) {
	keyToTenant := map[string]string{"secret-1": "acme", "secret-2": "globex"}

	r := gin.New()
	r.Use(APIKeyAuth("X-API-Key", keyToTenant))
	// Echo the resolved tenant so the test can assert tenant resolution.
	r.GET("/protected", func(c *gin.Context) { c.String(http.StatusOK, c.GetString("tenant")) })

	tests := []struct {
		name       string
		header     string
		value      string
		wantStatus int
		wantTenant string
	}{
		{"valid key resolves acme", "X-API-Key", "secret-1", http.StatusOK, "acme"},
		{"second key resolves globex", "X-API-Key", "secret-2", http.StatusOK, "globex"},
		{"valid Bearer token", "Authorization", "Bearer secret-1", http.StatusOK, "acme"},
		{"missing key", "", "", http.StatusUnauthorized, ""},
		{"wrong key", "X-API-Key", "nope", http.StatusUnauthorized, ""},
		{"empty key value", "X-API-Key", "", http.StatusUnauthorized, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && w.Body.String() != tt.wantTenant {
				t.Errorf("tenant: got %q, want %q", w.Body.String(), tt.wantTenant)
			}
		})
	}
}

func TestMatchAPIKey(t *testing.T) {
	keyToTenant := map[string]string{"aaa": "t1", "bbb": "t2"}

	if tenant, ok := matchAPIKey("aaa", keyToTenant); !ok || tenant != "t1" {
		t.Errorf("aaa: got (%q, %v), want (t1, true)", tenant, ok)
	}
	if tenant, ok := matchAPIKey("bbb", keyToTenant); !ok || tenant != "t2" {
		t.Errorf("bbb: got (%q, %v), want (t2, true)", tenant, ok)
	}
	if _, ok := matchAPIKey("ccc", keyToTenant); ok {
		t.Error("ccc should not match")
	}
	if _, ok := matchAPIKey("", keyToTenant); ok {
		t.Error("empty key should not match")
	}
	if _, ok := matchAPIKey("anything", nil); ok {
		t.Error("no configured keys should never match")
	}
}
