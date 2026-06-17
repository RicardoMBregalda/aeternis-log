package models

// HealthResponse represents the health check response
type HealthResponse struct {
	Status      string                 `json:"status"`
	Version     string                 `json:"version"`
	BuildTime   string                 `json:"build_time"`
	Timestamp   string                 `json:"timestamp"`
	Services    map[string]interface{} `json:"services"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
