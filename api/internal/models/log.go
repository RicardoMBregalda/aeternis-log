package models

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	LogLevelDebug    LogLevel = "DEBUG"
	LogLevelInfo     LogLevel = "INFO"
	LogLevelWarning  LogLevel = "WARNING"
	LogLevelError    LogLevel = "ERROR"
	LogLevelCritical LogLevel = "CRITICAL"
)

// Log represents a log entry in the system
type Log struct {
	ID         string                 `json:"id" bson:"id" binding:"required"`
	Hash       string                 `json:"hash" bson:"hash"`
	Timestamp  string                 `json:"timestamp" bson:"timestamp" binding:"required"`
	Source     string                 `json:"source" bson:"source" binding:"required"`
	Level      LogLevel               `json:"level" bson:"level" binding:"required"`
	Message    string                 `json:"message" bson:"message" binding:"required"`
	Metadata   map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
	Stacktrace string                 `json:"stacktrace,omitempty" bson:"stacktrace,omitempty"`
	CreatedAt  FlexTime               `json:"created_at" bson:"created_at"`
	BatchID    string                 `json:"batch_id,omitempty" bson:"batch_id,omitempty"`
	MerkleRoot string                 `json:"merkle_root,omitempty" bson:"merkle_root,omitempty"`
	BatchedAt  *FlexTime              `json:"batched_at,omitempty" bson:"batched_at,omitempty"`
}

// FlexTime is a flexible time type that can parse multiple formats
type FlexTime struct {
	time.Time
}

// UnmarshalBSONValue implements bsoncodec.ValueUnmarshaler for flexible time parsing
func (ft *FlexTime) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	// Handle BSON DateTime
	if t == bsontype.DateTime {
		// BSON DateTime is 8 bytes (int64 milliseconds since epoch)
		if len(data) != 8 {
			return fmt.Errorf("invalid BSON DateTime length: %d", len(data))
		}
		ms := int64(data[0]) | int64(data[1])<<8 | int64(data[2])<<16 | int64(data[3])<<24 |
			int64(data[4])<<32 | int64(data[5])<<40 | int64(data[6])<<48 | int64(data[7])<<56
		ft.Time = time.Unix(ms/1000, (ms%1000)*1000000).UTC()
		return nil
	}

	// Handle string
	if t == bsontype.String {
		// BSON strings are: 4 bytes length + string bytes + 1 null byte
		if len(data) < 5 {
			return fmt.Errorf("invalid BSON string length: %d", len(data))
		}
		strLen := int(data[0]) | int(data[1])<<8 | int(data[2])<<16 | int(data[3])<<24
		str := string(data[4 : 4+strLen-1]) // -1 to exclude null terminator

		// Try formats in order of likelihood
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.999999",     // Without timezone
			"2006-01-02T15:04:05",            // Without milliseconds
			"2006-01-02 15:04:05.999999",     // Space separator
			"2006-01-02 15:04:05",            // Space separator without ms
		}

		for _, format := range formats {
			if parsedTime, err := time.Parse(format, str); err == nil {
				// If parsed without timezone, assume UTC
				if parsedTime.Location() == time.UTC {
					ft.Time = parsedTime.UTC()
				} else {
					ft.Time = parsedTime
				}
				return nil
			}
		}

		return fmt.Errorf("unable to parse time string: %s", str)
	}

	return fmt.Errorf("unsupported BSON type %v for FlexTime", t)
}

// MarshalBSONValue implements bsoncodec.ValueMarshaler
func (ft FlexTime) MarshalBSONValue() (bsontype.Type, []byte, error) {
	// Convert time to BSON DateTime (milliseconds since epoch)
	dt := primitive.NewDateTimeFromTime(ft.Time)
	
	// Encode the DateTime as raw bytes (8 bytes, int64)
	ms := int64(dt)
	data := make([]byte, 8)
	data[0] = byte(ms)
	data[1] = byte(ms >> 8)
	data[2] = byte(ms >> 16)
	data[3] = byte(ms >> 24)
	data[4] = byte(ms >> 32)
	data[5] = byte(ms >> 40)
	data[6] = byte(ms >> 48)
	data[7] = byte(ms >> 56)
	
	return bsontype.DateTime, data, nil
}

// MarshalBSON implements custom BSON marshaling
func (ft FlexTime) MarshalBSON() ([]byte, error) {
	// For document-level marshaling, just use the primitive DateTime
	return bson.Marshal(bson.M{"$date": primitive.NewDateTimeFromTime(ft.Time)})
}

// MarshalJSON implements custom JSON marshaling
func (ft FlexTime) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, ft.Time.Format(time.RFC3339))), nil
}

// CreateLogRequest represents the request body for creating a new log
type CreateLogRequest struct {
	ID         string                 `json:"id,omitempty"`
	Timestamp  string                 `json:"timestamp,omitempty"`
	Source     string                 `json:"source" binding:"required"`
	Level      LogLevel               `json:"level" binding:"required,oneof=DEBUG INFO WARNING ERROR CRITICAL"`
	Message    string                 `json:"message" binding:"required"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Stacktrace string                 `json:"stacktrace,omitempty"`
}

// CreateLogResponse represents the response after creating a log
type CreateLogResponse struct {
	ID             string `json:"id"`
	Hash           string `json:"hash"`
	Status         string `json:"status"`
	MongoDBStatus  string `json:"mongodb_status"`
	FabricSync     string `json:"fabric_sync"`
	Durability     string `json:"durability"`
	Message        string `json:"message"`
}

// LogFilter represents query filters for searching logs
type LogFilter struct {
	Source string   `form:"source"`
	Level  LogLevel `form:"level"`
	Limit  int      `form:"limit,default=100"`
	Offset int      `form:"offset,default=0"`
}

// LogsResponse represents the response for listing logs
type LogsResponse struct {
	Logs   []*Log `json:"logs"`
	Cached bool   `json:"cached"`
	Count  int    `json:"count"`
}

// CalculateHash generates SHA256 hash of the log content
func (l *Log) CalculateHash() string {
	// Build content string matching Python implementation
	content := fmt.Sprintf("%s%s%s%s%s",
		l.ID,
		l.Timestamp,
		l.Source,
		l.Level,
		l.Message,
	)

	// Add metadata if present (sorted JSON for consistency)
	if len(l.Metadata) > 0 {
		metadataJSON, err := json.Marshal(l.Metadata)
		if err == nil {
			content += string(metadataJSON)
		}
	}

	// Add stacktrace if present
	if l.Stacktrace != "" {
		content += l.Stacktrace
	}

	// Calculate SHA256
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// Validate checks if the log has all required fields
func (l *Log) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("id is required")
	}
	if l.Source == "" {
		return fmt.Errorf("source is required")
	}
	if l.Level == "" {
		return fmt.Errorf("level is required")
	}
	if l.Message == "" {
		return fmt.Errorf("message is required")
	}

	// Validate log level
	validLevels := map[LogLevel]bool{
		LogLevelDebug:    true,
		LogLevelInfo:     true,
		LogLevelWarning:  true,
		LogLevelError:    true,
		LogLevelCritical: true,
	}

	if !validLevels[l.Level] {
		return fmt.Errorf("invalid log level: %s", l.Level)
	}

	return nil
}

// ToMap converts Log to map for flexible serialization
func (l *Log) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"id":         l.ID,
		"hash":       l.Hash,
		"timestamp":  l.Timestamp,
		"source":     l.Source,
		"level":      l.Level,
		"message":    l.Message,
		"created_at": l.CreatedAt,
	}

	if len(l.Metadata) > 0 {
		m["metadata"] = l.Metadata
	}
	if l.Stacktrace != "" {
		m["stacktrace"] = l.Stacktrace
	}
	if l.BatchID != "" {
		m["batch_id"] = l.BatchID
	}
	if l.MerkleRoot != "" {
		m["merkle_root"] = l.MerkleRoot
	}
	if l.BatchedAt != nil {
		m["batched_at"] = l.BatchedAt
	}

	return m
}

// NewLog creates a new Log with defaults
func NewLog(source, level, message string) *Log {
	now := time.Now().UTC()
	log := &Log{
		Source:    source,
		Level:     LogLevel(level),
		Message:   message,
		Timestamp: now.Format(time.RFC3339),
		CreatedAt: FlexTime{Time: now},
		Metadata:  make(map[string]interface{}),
	}
	return log
}

// MarshalBSON is removed - use default BSON marshaling
// The custom implementation was causing id field conflicts
