package models

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
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

// Integrity outcomes reported by batch verification.
const (
	IntegrityValid      = "VALID"      // recomputed root matches the authoritative anchor
	IntegrityCorrupted  = "CORRUPTED"  // recomputed root does NOT match the anchor
	IntegrityUnanchored = "UNANCHORED" // the batch is not (yet) anchored on the ledger
)

// Anchor status: where the authoritative Merkle root was (or wasn't) read from.
const (
	AnchorAnchored   = "ANCHORED" // root read from the on-chain ledger (authoritative)
	AnchorUnanchored = "UNANCHORED"
	AnchorUnknown    = "UNKNOWN" // ledger unavailable or sync disabled — on-chain proof not consulted
)

// VerifyBatchResponse represents the response from batch verification.
//
// Integrity is decided against OnChainMerkleRoot when AnchorStatus == ANCHORED
// (the ledger is the source of truth). When the ledger cannot be consulted
// (AnchorStatus == UNKNOWN) the result falls back to a local-consistency check
// between OriginalMerkleRoot and RecalculatedMerkleRoot, and the caller is told
// the on-chain anchor was not read.
type VerifyBatchResponse struct {
	BatchID                string `json:"batch_id"`
	IsValid                bool   `json:"is_valid"`
	NumLogs                int    `json:"num_logs"`
	OriginalMerkleRoot     string `json:"original_merkle_root"`
	RecalculatedMerkleRoot string `json:"recalculated_merkle_root"`
	OnChainMerkleRoot      string `json:"on_chain_merkle_root,omitempty"`
	AnchorStatus           string `json:"anchor_status"`
	Integrity              string `json:"integrity"`
	Message                string `json:"message"`
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

// Merkle tree domain separators (v2): leaves and internal nodes are hashed with
// distinct prefixes so a precomputed internal node can never be passed off as a
// leaf (and vice versa).
const (
	merkleLeafPrefix byte = 0x00
	merkleNodePrefix byte = 0x01
)

// writeLenPrefixed writes len(s) as 8 big-endian bytes followed by s, so that
// concatenated fields have unambiguous boundaries in the hashed pre-image.
func writeLenPrefixed(h hash.Hash, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	h.Write(n[:])
	h.Write([]byte(s))
}

// CombineHashesV2 combines two child hashes into an internal Merkle node, tagged
// with the internal-node domain separator so it cannot be confused with a leaf.
func CombineHashesV2(left, right string) string {
	h := sha256.New()
	h.Write([]byte{merkleNodePrefix})
	h.Write([]byte(left))
	h.Write([]byte(right))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// BuildMerkleTreeV2 builds the Merkle root with internal-node domain separation
// and promotes an odd trailing node up a level instead of duplicating it — the
// duplicate-last shortcut is the CVE-2012-2459 second-preimage weakness.
func BuildMerkleTreeV2(hashes []string) string {
	if len(hashes) == 0 {
		return ""
	}
	level := make([]string, len(hashes))
	copy(level, hashes)
	for len(level) > 1 {
		next := make([]string, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 < len(level) {
				next = append(next, CombineHashesV2(level[i], level[i+1]))
			} else {
				next = append(next, level[i]) // promote odd node (no duplication)
			}
		}
		level = next
	}
	return level[0]
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
