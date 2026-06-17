package models

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Anchor lifecycle states for a batched record (see Record.AnchorStatus).
const (
	RecordAnchorPending  = "pending"  // claimed into a batch, not yet anchored
	RecordAnchorAnchored = "anchored" // batch committed on the ledger
	RecordAnchorFailed   = "failed"   // anchor attempt failed; reconciler retries
)

// Record is the generic, domain-agnostic auditable record at the core of the
// Tamper-Evident Data Anchoring pattern. A Log is just a Record in the "logs"
// domain; any client can anchor arbitrary records under their own domain.
type Record struct {
	Tenant    string                 `json:"tenant" bson:"tenant"`
	Domain    string                 `json:"domain" bson:"domain"`
	ID        string                 `json:"id" bson:"id"`
	Timestamp string                 `json:"timestamp" bson:"timestamp"`
	Source    string                 `json:"source" bson:"source"`
	Payload   map[string]interface{} `json:"payload" bson:"payload"`

	// HashFields optionally restricts which payload keys feed the integrity hash
	// (the rest of the payload can change without breaking the hash). Empty means
	// the whole payload. It is stored so the hash stays reproducible on verify.
	HashFields []string `json:"hash_fields,omitempty" bson:"hash_fields,omitempty"`
	Hash       string   `json:"hash" bson:"hash"`

	CreatedAt  FlexTime  `json:"created_at" bson:"created_at"`
	BatchID    string    `json:"batch_id,omitempty" bson:"batch_id,omitempty"`
	MerkleRoot string    `json:"merkle_root,omitempty" bson:"merkle_root,omitempty"`
	// AnchorStatus is the batch's anchoring lifecycle: pending (claimed, not yet
	// anchored), anchored (committed on the ledger), or failed (anchor attempt
	// failed, awaiting reconciliation). Empty on never-batched records.
	AnchorStatus string `json:"anchor_status,omitempty" bson:"anchor_status,omitempty"`
	// TxID is the Fabric transaction that anchored the batch (set after anchoring).
	TxID       string    `json:"tx_id,omitempty" bson:"tx_id,omitempty"`
	BatchedAt  *FlexTime `json:"batched_at,omitempty" bson:"batched_at,omitempty"`
	// DeletedAt marks a soft-deleted record; excluded from the hash so deleting
	// never invalidates the integrity proof or the on-chain anchor.
	DeletedAt *FlexTime `json:"deleted_at,omitempty" bson:"deleted_at,omitempty"`
}

// canonicalPayload returns a deterministic JSON serialization of the payload (or
// only the HashFields keys). encoding/json sorts map keys, so the output is
// stable for equal content regardless of insertion order.
func (r *Record) canonicalPayload() string {
	src := r.Payload
	if len(r.HashFields) > 0 {
		sub := make(map[string]interface{}, len(r.HashFields))
		for _, k := range r.HashFields {
			if v, ok := r.Payload[k]; ok {
				sub[k] = v
			}
		}
		src = sub
	}
	b, err := json.Marshal(src)
	if err != nil {
		return ""
	}
	return string(b)
}

// CalculateHash computes the SHA-256 integrity hash over the record's identity
// fields and its canonical payload (optionally restricted to HashFields).
func (r *Record) CalculateHash() string {
	content := r.ID + r.Timestamp + r.Source + r.canonicalPayload()
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// Validate checks the required fields of a record.
func (r *Record) Validate() error {
	if r.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	if r.Source == "" {
		return fmt.Errorf("source is required")
	}
	if r.Payload == nil {
		return fmt.Errorf("payload is required")
	}
	return nil
}

// CreateRecordRequest is the request body for creating a record.
type CreateRecordRequest struct {
	ID         string                 `json:"id,omitempty"`
	Timestamp  string                 `json:"timestamp,omitempty"`
	Source     string                 `json:"source" binding:"required"`
	Payload    map[string]interface{} `json:"payload" binding:"required"`
	HashFields []string               `json:"hash_fields,omitempty"`
}

// ListRecordsResponse is the response for listing records (cursor or offset).
type ListRecordsResponse struct {
	Records    []*Record `json:"records"`
	Total      int       `json:"total"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// CalculateRecordMerkleRoot recomputes each record's hash from its content and
// builds the Merkle root, returning the root and the leaf hashes. Recomputing
// (instead of trusting the stored hash) is what makes the batch tamper-evident:
// if a record's content changed, its leaf hash — and the root — change too.
func CalculateRecordMerkleRoot(records []*Record) (string, []string) {
	hashes := make([]string, len(records))
	for i, r := range records {
		hashes[i] = r.CalculateHash()
	}
	return BuildMerkleTree(hashes), hashes
}

// RecordBatchResult describes a Merkle batch of records anchored to Fabric.
type RecordBatchResult struct {
	BatchID    string `json:"batch_id"`
	Tenant     string `json:"tenant"`
	Domain     string `json:"domain"`
	MerkleRoot string `json:"merkle_root"`
	NumRecords int    `json:"num_records"`
	TxID       string `json:"tx_id,omitempty"`
	Anchored   bool   `json:"anchored"`
	// Channel is the Fabric channel the batch was anchored to (the tenant's
	// dedicated channel when mapped, else the default).
	Channel string `json:"channel,omitempty"`
}

// AuditBatch summarizes one anchored batch for an audit report.
type AuditBatch struct {
	BatchID    string `json:"batch_id" bson:"_id"`
	MerkleRoot string `json:"merkle_root" bson:"merkle_root"`
	TxID       string `json:"tx_id" bson:"tx_id"`
	NumRecords int    `json:"num_records" bson:"num_records"`
	BatchedAt  string `json:"batched_at" bson:"batched_at"`
}

// AuditReport is the data behind a tenant/domain audit report over a time range.
type AuditReport struct {
	Tenant      string       `json:"tenant"`
	Domain      string       `json:"domain"`
	From        string       `json:"from"`
	To          string       `json:"to"`
	GeneratedAt string       `json:"generated_at"`
	Batches     []AuditBatch `json:"batches"`
	TotalBatches int         `json:"total_batches"`
	TotalRecords int         `json:"total_records"`
}
