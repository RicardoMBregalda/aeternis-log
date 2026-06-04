package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAPIKeyAuth(t *testing.T) {
	keys := []string{"secret-1", "secret-2"}

	r := gin.New()
	r.Use(APIKeyAuth("X-API-Key", keys))
	r.GET("/protected", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	tests := []struct {
		name       string
		header     string
		value      string
		wantStatus int
	}{
		{"valid X-API-Key", "X-API-Key", "secret-1", http.StatusOK},
		{"valid second key", "X-API-Key", "secret-2", http.StatusOK},
		{"valid Bearer token", "Authorization", "Bearer secret-1", http.StatusOK},
		{"missing key", "", "", http.StatusUnauthorized},
		{"wrong key", "X-API-Key", "nope", http.StatusUnauthorized},
		{"empty key value", "X-API-Key", "", http.StatusUnauthorized},
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
		})
	}
}

func TestRateLimiter(t *testing.T) {
	r := gin.New()
	r.Use(RateLimiter(2, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// A distinct IP keeps this test isolated from the package-global limiter map.
	const ip = "203.0.113.7:5555"
	do := func() int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	for i := 1; i <= 2; i++ {
		if code := do(); code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, code)
		}
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("request 3: got %d, want 429", code)
	}
}

func TestMatchAPIKeyConstantTime(t *testing.T) {
	keys := []string{"aaa", "bbb"}
	cases := map[string]bool{
		"aaa": true,
		"bbb": true,
		"ccc": false,
		"":    false,
	}
	for in, want := range cases {
		if got := matchAPIKey(in, keys); got != want {
			t.Errorf("matchAPIKey(%q) = %v, want %v", in, got, want)
		}
	}
	if matchAPIKey("anything", nil) {
		t.Error("matchAPIKey with no configured keys should never match")
	}
}
