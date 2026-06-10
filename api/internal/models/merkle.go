package models

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// MerkleBatch represents a batch of logs with Merkle Tree root
type MerkleBatch struct {
	BatchID    string    `json:"batch_id" bson:"batch_id"`
	MerkleRoot string    `json:"merkle_root" bson:"merkle_root"`
	Timestamp  string    `json:"timestamp" bson:"timestamp"`
	NumLogs    int       `json:"num_logs" bson:"num_logs"`
	LogIDs     []string  `json:"log_ids" bson:"log_ids"`
	CreatedAt  time.Time `json:"created_at,omitempty" bson:"created_at,omitempty"`
}

// MerkleNode represents a node in the Merkle Tree
type MerkleNode struct {
	Hash  string       `json:"hash"`
	Left  *MerkleNode  `json:"left,omitempty"`
	Right *MerkleNode  `json:"right,omitempty"`
}

// CreateBatchRequest represents the request to create a Merkle batch
type CreateBatchRequest struct {
	BatchSize int `json:"batch_size,omitempty" form:"batch_size" default:"100"`
}

// CreateBatchResponse represents the response after creating a batch
type CreateBatchResponse struct {
	BatchID    string   `json:"batch_id"`
	MerkleRoot string   `json:"merkle_root"`
	NumLogs    int      `json:"num_logs"`
	LogIDs     []string `json:"log_ids"`
	Status     string   `json:"status"`
	Message    string   `json:"message"`
}

// VerifyBatchResponse represents the response from batch verification
type VerifyBatchResponse struct {
	BatchID               string `json:"batch_id"`
	IsValid               bool   `json:"is_valid"`
	NumLogs               int    `json:"num_logs"`
	OriginalMerkleRoot    string `json:"original_merkle_root"`
	RecalculatedMerkleRoot string `json:"recalculated_merkle_root"`
	Integrity             string `json:"integrity"`
	Message               string `json:"message"`
}

// ListBatchesResponse represents the response for listing batches
type ListBatchesResponse struct {
	Batches      []*BatchInfo `json:"batches"`
	TotalBatches int          `json:"total_batches"`
}

// GetBatchResponse represents the response for getting a specific batch
type GetBatchResponse struct {
	Batch   *MerkleBatch `json:"batch"`
	Logs    []*Log       `json:"logs"`
	NumLogs int          `json:"num_logs"`
}

// NewMerkleBatch creates a new MerkleBatch
func NewMerkleBatch(batchID, merkleRoot string, numLogs int, logIDs []string) *MerkleBatch {
	now := time.Now().UTC()
	return &MerkleBatch{
		BatchID:    batchID,
		MerkleRoot: merkleRoot,
		Timestamp:  now.Format(time.RFC3339),
		NumLogs:    numLogs,
		LogIDs:     logIDs,
		CreatedAt:  now,
	}
}

// CalculateLogHash calculates SHA256 hash of a log for Merkle Tree
func CalculateLogHash(log *Log) string {
	// Build content string matching Python implementation
	content := fmt.Sprintf("%s%s%s%s%s",
		log.ID,
		log.Timestamp,
		log.Source,
		log.Level,
		log.Message,
	)

	// Add metadata if present
	if len(log.Metadata) > 0 {
		metadataJSON, err := json.Marshal(log.Metadata)
		if err == nil {
			content += string(metadataJSON)
		}
	}

	// Add stacktrace if present
	if log.Stacktrace != "" {
		content += log.Stacktrace
	}

	// Calculate SHA256
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// CombineHashes combines two hashes using SHA256
func CombineHashes(hash1, hash2 string) string {
	combined := hash1 + hash2
	hash := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", hash)
}

// BuildMerkleTree builds a Merkle Tree from a list of hashes and returns the root
func BuildMerkleTree(hashes []string) string {
	if len(hashes) == 0 {
		return ""
	}

	if len(hashes) == 1 {
		return hashes[0]
	}

	// Copy hashes to avoid modifying original
	currentLevel := make([]string, len(hashes))
	copy(currentLevel, hashes)

	// Build tree bottom-up
	for len(currentLevel) > 1 {
		var nextLevel []string

		// If odd number of nodes, duplicate the last one
		if len(currentLevel)%2 != 0 {
			currentLevel = append(currentLevel, currentLevel[len(currentLevel)-1])
		}

		// Combine pairs of hashes
		for i := 0; i < len(currentLevel); i += 2 {
			combined := CombineHashes(currentLevel[i], currentLevel[i+1])
			nextLevel = append(nextLevel, combined)
		}

		currentLevel = nextLevel
	}

	return currentLevel[0]
}

// CalculateMerkleRoot calculates the Merkle root from a list of logs
func CalculateMerkleRoot(logs []*Log) (string, []string) {
	hashes := make([]string, len(logs))
	for i, log := range logs {
		hashes[i] = CalculateLogHash(log)
	}
	merkleRoot := BuildMerkleTree(hashes)
	return merkleRoot, hashes
}

// Validate validates the MerkleBatch
func (mb *MerkleBatch) Validate() error {
	if mb.BatchID == "" {
		return fmt.Errorf("batch_id is required")
	}
	if mb.MerkleRoot == "" {
		return fmt.Errorf("merkle_root is required")
	}
	if mb.NumLogs <= 0 {
		return fmt.Errorf("num_logs must be greater than 0")
	}
	if len(mb.LogIDs) != mb.NumLogs {
		return fmt.Errorf("log_ids length (%d) does not match num_logs (%d)", len(mb.LogIDs), mb.NumLogs)
	}
	return nil
}

// ToFabricArgs converts MerkleBatch to arguments for Fabric chaincode
func (mb *MerkleBatch) ToFabricArgs() []string {
	logIDsJSON, _ := json.Marshal(mb.LogIDs)
	return []string{
		mb.BatchID,
		mb.MerkleRoot,
		mb.Timestamp,
		fmt.Sprintf("%d", mb.NumLogs),
		string(logIDsJSON),
	}
}
