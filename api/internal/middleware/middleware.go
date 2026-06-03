package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/logger"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/gin-gonic/gin"
)

// CORS middleware for handling Cross-Origin Resource Sharing
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
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
