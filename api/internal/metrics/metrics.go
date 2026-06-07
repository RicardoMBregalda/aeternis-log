// Package metrics exposes Prometheus instrumentation for the API: HTTP request
// counters/latency, Go runtime + process collectors, and product counters for
// anchored batches/records. It owns a private registry served via Handler() on the
// dedicated metrics port (cfg.Metrics).
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry = prometheus.NewRegistry()

	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route template and status code.",
		},
		[]string{"method", "route", "status"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds by method and route template.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	batchesAnchored = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "batches_anchored_total",
			Help: "Merkle batches anchored to Fabric by tenant and domain.",
		},
		[]string{"tenant", "domain"},
	)

	recordsAnchored = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "records_anchored_total",
			Help: "Records anchored to Fabric (summed over batches) by tenant and domain.",
		},
		[]string{"tenant", "domain"},
	)

	integrityVerifications = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "integrity_verifications_total",
			Help: "Batch integrity verifications by domain and result (VALID or CORRUPTED).",
		},
		[]string{"domain", "result"},
	)
)

func init() {
	registry.MustRegister(
		httpRequests,
		httpDuration,
		batchesAnchored,
		recordsAnchored,
		integrityVerifications,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Handler serves the Prometheus exposition format for the private registry.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// Middleware records request count and latency. It labels by the matched route
// template (c.FullPath()) — not the raw path — to keep label cardinality bounded.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		httpRequests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
		httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

// RecordAnchoredBatch bumps the product counters when a batch is anchored to Fabric.
func RecordAnchoredBatch(tenant, domain string, numRecords int) {
	if tenant == "" {
		tenant = "default"
	}
	batchesAnchored.WithLabelValues(tenant, domain).Inc()
	recordsAnchored.WithLabelValues(tenant, domain).Add(float64(numRecords))
}

// RecordVerification records the outcome of a batch integrity check. A rising
// CORRUPTED count means a Merkle-root discrepancy was detected — alert on it.
func RecordVerification(domain string, valid bool) {
	result := "CORRUPTED"
	if valid {
		result = "VALID"
	}
	integrityVerifications.WithLabelValues(domain, result).Inc()
}
