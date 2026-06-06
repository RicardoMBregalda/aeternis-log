package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics handler returned %d", rec.Code)
	}
	return rec.Body.String()
}

func TestHandlerExposesProductCounters(t *testing.T) {
	RecordAnchoredBatch("acme", "audit", 3)
	body := scrape(t)
	for _, want := range []string{"batches_anchored_total", "records_anchored_total", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q", want)
		}
	}
	// Prometheus sorts labels alphabetically (domain before tenant).
	if !strings.Contains(body, `batches_anchored_total{domain="audit",tenant="acme"} 1`) {
		t.Errorf("expected acme/audit batch counter == 1, body:\n%s", body)
	}
	if !strings.Contains(body, `records_anchored_total{domain="audit",tenant="acme"} 3`) {
		t.Errorf("expected acme/audit records counter == 3, body:\n%s", body)
	}
}

func TestMiddlewareRecordsRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	body := scrape(t)
	if !strings.Contains(body, `http_requests_total{method="GET",route="/ping",status="200"}`) {
		t.Errorf("expected http_requests_total sample for /ping, body:\n%s", body)
	}
}
