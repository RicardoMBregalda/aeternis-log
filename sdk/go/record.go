// Package anchor is a Go client for the Tamper-Evident Data Anchoring API.
//
// Besides wrapping the HTTP API (with automatic retries), it recomputes record
// hashes and Merkle roots locally and independently of the server, so an auditor
// can verify integrity without trusting the API.
package aeternislog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Record mirrors a record returned by the API.
type Record struct {
	Domain     string                 `json:"domain"`
	ID         string                 `json:"id"`
	Timestamp  string                 `json:"timestamp"`
	Source     string                 `json:"source"`
	Payload    map[string]interface{} `json:"payload"`
	HashFields []string               `json:"hash_fields,omitempty"`
	Hash       string                 `json:"hash"`
	BatchID    string                 `json:"batch_id,omitempty"`
	MerkleRoot string                 `json:"merkle_root,omitempty"`
}

// ComputeHash recomputes the record's integrity hash locally, independently of
// the server: SHA-256(id | timestamp | source | canonical(payload)). This is what
// makes verification trustless — the client never relies on the server's hash.
func (r *Record) ComputeHash() string {
	content := r.ID + r.Timestamp + r.Source + canonicalPayload(r.Payload, r.HashFields)
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// canonicalPayload serializes the payload (or only the HashFields keys)
// deterministically. encoding/json sorts map keys, so the output is stable.
func canonicalPayload(payload map[string]interface{}, hashFields []string) string {
	src := payload
	if len(hashFields) > 0 {
		sub := make(map[string]interface{}, len(hashFields))
		for _, k := range hashFields {
			if v, ok := payload[k]; ok {
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

// MerkleRoot recomputes the Merkle root of an ordered set of records locally.
func MerkleRoot(records []*Record) string {
	hashes := make([]string, len(records))
	for i, r := range records {
		hashes[i] = r.ComputeHash()
	}
	return buildMerkleTree(hashes)
}

func buildMerkleTree(hashes []string) string {
	if len(hashes) == 0 {
		return ""
	}
	level := append([]string(nil), hashes...)
	for len(level) > 1 {
		if len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}
		next := make([]string, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			sum := sha256.Sum256([]byte(level[i] + level[i+1]))
			next = append(next, hex.EncodeToString(sum[:]))
		}
		level = next
	}
	return level[0]
}

// VerifyRecordsLocally recomputes the Merkle root of the given (ordered) records
// and compares it with an expected root — e.g. the one anchored on the blockchain.
// Returns true only if every record's content still hashes to the anchored root.
func VerifyRecordsLocally(records []*Record, expectedRoot string) bool {
	return MerkleRoot(records) == expectedRoot
}
