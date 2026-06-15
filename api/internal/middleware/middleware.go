package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/logger"
	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	corsAllowHeaders = "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-Key"
	corsAllowMethods = "POST, OPTIONS, GET, PUT, DELETE, PATCH"
)

// CORS returns a CORS middleware restricted to an allowlist of origins. A
// configured "*" allows any origin but WITHOUT credentials — the invalid
// "*" + Allow-Credentials combination (rejected by browsers) is never emitted.
// An explicit origin match reflects that origin and enables credentialed CORS.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowAll := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		switch o = strings.TrimSpace(o); {
		case o == "*":
			allowAll = true
		case o != "":
			allowed[o] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		switch {
		case allowAll:
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		case origin != "":
			if _, ok := allowed[origin]; ok {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Add("Vary", "Origin")
			}
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		c.Writer.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// MaxBodyBytes caps the request body size to prevent memory-exhaustion DoS.
// Reading a body larger than the limit fails, which handlers surface as a
// 400 (invalid request). A non-positive limit disables the cap.
func MaxBodyBytes(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// domainPattern is a conservative, DNS-label-like token: lowercase
// alphanumerics and hyphens, starting alphanumeric, up to 63 chars.
var domainPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidateDomain rejects requests whose :domain path parameter is not a safe
// token, guarding every domain-scoped route from odd values flowing into Mongo
// filters and on-chain batch ids.
func ValidateDomain() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !domainPattern.MatchString(c.Param("domain")) {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_domain",
				Message: "domain must match ^[a-z0-9][a-z0-9-]{0,62}$",
				Code:    http.StatusBadRequest,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequestID middleware adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Set("request_id", requestID)
		
		c.Next()
	}
}

// Simple in-memory rate limiter
var (
	rateLimitMap   = make(map[string][]time.Time)
	rateLimitMutex sync.RWMutex
)

// RateLimiter middleware for basic rate limiting (simple in-memory version)
// Limits to maxRequests per window duration per IP
func RateLimiter(maxRequests int, window time.Duration) gin.HandlerFunc {
	// This is a simplified version. In production, use a proper rate limiter like
	// github.com/ulule/limiter with Redis backend
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		now := time.Now()

		rateLimitMutex.Lock()
		defer rateLimitMutex.Unlock()

		// Clean old entries
		if timestamps, exists := rateLimitMap[clientIP]; exists {
			var validTimestamps []time.Time
			for _, ts := range timestamps {
				if now.Sub(ts) < window {
					validTimestamps = append(validTimestamps, ts)
				}
			}
			rateLimitMap[clientIP] = validTimestamps
		}

		// Check rate limit
		if len(rateLimitMap[clientIP]) >= maxRequests {
			c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error:   "rate_limit_exceeded",
				Message: fmt.Sprintf("Rate limit exceeded: %d requests per %s", maxRequests, window),
				Code:    http.StatusTooManyRequests,
			})
			c.Abort()
			return
		}

		// Add current request
		rateLimitMap[clientIP] = append(rateLimitMap[clientIP], now)

		c.Next()
	}
}

// APIKeyAuth authenticates requests against the configured API keys and resolves
// the caller's tenant. The key may be sent in the configured header (e.g.
// X-API-Key) or as a Bearer token in the Authorization header. On success the
// tenant is stored in the context ("tenant"). Keys are compared in constant time.
func APIKeyAuth(headerName string, keyToTenant map[string]string) gin.HandlerFunc {
	return func(c *gin.Context) {
		presented := c.GetHeader(headerName)
		if presented == "" {
			if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				presented = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}

		tenant, ok := matchAPIKey(presented, keyToTenant)
		if presented == "" || !ok {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "missing or invalid API key",
				Code:    http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		c.Set("tenant", tenant)
		c.Next()
	}
}

// matchAPIKey returns the tenant for a presented key. It checks every configured
// key (no early return) with a constant-time comparison so response timing does
// not leak key contents. A configured key may be a plaintext value or a
// "sha256:<hex>" hash of the key — the latter lets operators avoid storing the
// raw key at rest (the presented key is hashed and compared in constant time).
func matchAPIKey(presented string, keyToTenant map[string]string) (string, bool) {
	sum := sha256.Sum256([]byte(presented))
	presentedHash := hex.EncodeToString(sum[:])

	tenant := ""
	match := false
	for key, t := range keyToTenant {
		var equal bool
		if h, ok := strings.CutPrefix(key, "sha256:"); ok {
			equal = subtle.ConstantTimeCompare([]byte(presentedHash), []byte(h)) == 1
		} else {
			equal = subtle.ConstantTimeCompare([]byte(presented), []byte(key)) == 1
		}
		if equal {
			tenant = t
			match = true
		}
	}
	return tenant, match
}

// Timeout middleware adds a timeout to requests
func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Replace request context
		c.Request = c.Request.WithContext(ctx)

		// Channel to signal completion
		finished := make(chan struct{})

		go func() {
			c.Next()
			close(finished)
		}()

		select {
		case <-finished:
			// Request completed successfully
			return
		case <-ctx.Done():
			// Timeout occurred
			c.JSON(http.StatusGatewayTimeout, models.ErrorResponse{
				Error:   "request_timeout",
				Message: fmt.Sprintf("Request timeout after %s", timeout),
				Code:    http.StatusGatewayTimeout,
			})
			c.Abort()
		}
	}
}

// generateRequestID generates a simple request ID
func generateRequestID() string {
	return time.Now().UTC().Format("20060102150405.000000")
}

// SecurityHeaders adds security-related headers
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("X-Frame-Options", "DENY")
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Next()
	}
}

// ValidateContentType validates that the request content type is JSON for POST/PUT/PATCH
func ValidateContentType() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "POST" || method == "PUT" || method == "PATCH" {
			contentType := c.GetHeader("Content-Type")
			if contentType != "" && contentType != "application/json" {
				c.JSON(http.StatusUnsupportedMediaType, models.ErrorResponse{
					Error:   "invalid_content_type",
					Message: "Content-Type must be application/json",
					Code:    http.StatusUnsupportedMediaType,
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// RequestLogger logs every HTTP request as a structured event, enriched with
// the request ID set by RequestID. It must run after RequestID.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		log := logger.WithRequestID(c.GetString("request_id"))
		evt := log.Info()
		status := c.Writer.Status()
		switch {
		case status >= 500:
			evt = log.Error()
		case status >= 400:
			evt = log.Warn()
		}

		evt.
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Dur("latency", time.Since(start)).
			Str("client_ip", c.ClientIP()).
			Int("bytes", c.Writer.Size()).
			Msg("http request")
	}
}
