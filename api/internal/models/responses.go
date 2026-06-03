package models

// HealthResponse represents the health check response
type HealthResponse struct {
	Status      string                 `json:"status"`
	Version     string                 `json:"version"`
	BuildTime   string                 `json:"build_time"`
	Timestamp   string                 `json:"timestamp"`
	Services    map[string]interface{} `json:"services"`
}

// WALStatsResponse represents WAL statistics response
type WALStatsResponse struct {
	WALStatistics     map[string]interface{} `json:"wal_statistics"`
	Status            string                 `json:"status"`
	DataLossGuarantee string                 `json:"data_loss_guarantee"`
}

// StatsResponse represents system statistics response
type StatsResponse struct {
	TotalLogs             int                    `json:"total_logs"`
	FabricSyncStatus      map[string]int         `json:"fabric_sync_status"`
	OptimizationsActive   bool                   `json:"optimizations_active"`
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

// ListLogsResponse represents the response for listing logs
type ListLogsResponse struct {
	Logs   []*Log `json:"logs"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}
