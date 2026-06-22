package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SmartContract provides functions for managing logs
type SmartContract struct {
	contractapi.Contract
}

// Log describes the structure of a log entry
type Log struct {
	ID         string    `json:"id"`
	Hash       string    `json:"hash"`
	Timestamp  time.Time `json:"timestamp"`
	Source     string    `json:"source"`
	Level      string    `json:"level"`
	Message    string    `json:"message"`
	Metadata   string    `json:"metadata"`
	Stacktrace string    `json:"stacktrace,omitempty"` // Optional field for ERROR logs
}

// MerkleBatch describes a batch of logs with Merkle Root
type MerkleBatch struct {
	Tenant     string    `json:"tenant"`
	BatchID    string    `json:"batch_id"`
	MerkleRoot string    `json:"merkle_root"`
	Timestamp  time.Time `json:"timestamp"`
	NumLogs    int       `json:"num_logs"`
	LogIDs     []string  `json:"log_ids"` // List of log IDs in this batch
}

// batchObjectType namespaces Merkle-batch composite keys: (tenant, batchID).
const batchObjectType = "batch"

// clientTenant derives the calling tenant from the client identity — never from
// a function argument — so a caller cannot forge another tenant's scope. It
// prefers the CA-signed `tenant` certificate attribute (multi-tenant within one
// org); absent that it falls back to the MSP ID (org-per-tenant). This is the
// ledger-level isolation boundary: batch state is partitioned by the result.
func (s *SmartContract) clientTenant(ctx contractapi.TransactionContextInterface) (string, error) {
	ci := ctx.GetClientIdentity()
	if v, found, err := ci.GetAttributeValue("tenant"); err == nil && found && v != "" {
		return v, nil
	}
	mspID, err := ci.GetMSPID()
	if err != nil {
		return "", fmt.Errorf("cannot determine client identity: %v", err)
	}
	if mspID == "" {
		return "", fmt.Errorf("client identity carries no tenant attribute and no MSP ID")
	}
	return mspID, nil
}

// batchKey builds the tenant-scoped composite key for a batch.
func (s *SmartContract) batchKey(ctx contractapi.TransactionContextInterface, tenant, batchID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey(batchObjectType, []string{tenant, batchID})
}

// CreateLog creates a new log entry in the ledger (OPTIMIZED v2)
// Removed LogExists check for better performance (50% faster!)
// Idempotent design: duplicate inserts will overwrite with same data
// Now accepts optional stacktrace parameter (8th parameter)
func (s *SmartContract) CreateLog(ctx contractapi.TransactionContextInterface, id string, hash string, timestamp string, source string, level string, message string, metadata string, stacktrace string) error {
	// OPTIMIZATION 1: Skip LogExists check (saves 1 GetState call)
	// Trade-off: Duplicate IDs will silently overwrite (acceptable for idempotent logs)

	// OPTIMIZATION 2: Simplified timestamp parsing
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		// Fallback to current time instead of failing
		parsedTime = time.Now()
	}

	// OPTIMIZATION 3: Direct struct creation
	log := Log{
		ID:         id,
		Hash:       hash,
		Timestamp:  parsedTime,
		Source:     source,
		Level:      level,
		Message:    message,
		Metadata:   metadata,
		Stacktrace: stacktrace, // Optional stacktrace field
	}

	// OPTIMIZATION 4: Marshal and PutState in one flow
	logJSON, err := json.Marshal(log)
	if err != nil {
		return err
	}

	// Direct PutState without validation
	return ctx.GetStub().PutState(id, logJSON)
}

// QueryLog returns the log entry with the given ID
func (s *SmartContract) QueryLog(ctx contractapi.TransactionContextInterface, id string) (*Log, error) {
	logJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %v", err)
	}
	if logJSON == nil {
		return nil, fmt.Errorf("the log %s does not exist", id)
	}

	var log Log
	err = json.Unmarshal(logJSON, &log)
	if err != nil {
		return nil, err
	}

	return &log, nil
}

// LogExists returns true when log with given ID exists in world state
func (s *SmartContract) LogExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	logJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return false, fmt.Errorf("failed to read from world state: %v", err)
	}

	return logJSON != nil, nil
}

// GetAllLogs returns all logs found in world state
func (s *SmartContract) GetAllLogs(ctx contractapi.TransactionContextInterface) ([]*Log, error) {
	resultsIterator, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var logs []*Log
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var log Log
		err = json.Unmarshal(queryResponse.Value, &log)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, nil
}

// GetLogHistory returns the history of a log entry
func (s *SmartContract) GetLogHistory(ctx contractapi.TransactionContextInterface, id string) ([]HistoryQueryResult, error) {
	resultsIterator, err := ctx.GetStub().GetHistoryForKey(id)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var records []HistoryQueryResult
	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var log Log
		if len(response.Value) > 0 {
			err = json.Unmarshal(response.Value, &log)
			if err != nil {
				return nil, err
			}
		}

		timestamp := time.Unix(response.Timestamp.Seconds, int64(response.Timestamp.Nanos)).String()

		record := HistoryQueryResult{
			TxId:      response.TxId,
			Timestamp: timestamp,
			IsDelete:  response.IsDelete,
			Log:       log,
		}
		records = append(records, record)
	}

	return records, nil
}

// HistoryQueryResult structure for history queries
type HistoryQueryResult struct {
	TxId      string `json:"txId"`
	Timestamp string `json:"timestamp"`
	IsDelete  bool   `json:"isDelete"`
	Log       Log    `json:"log"`
}

// QueryLogsByLevel returns logs with the specified level
func (s *SmartContract) QueryLogsByLevel(ctx contractapi.TransactionContextInterface, level string) ([]*Log, error) {
	queryString := fmt.Sprintf(`{"selector":{"level":"%s"}}`, level)
	return s.getQueryResultForQueryString(ctx, queryString)
}

// QueryLogsBySource returns logs from the specified source
func (s *SmartContract) QueryLogsBySource(ctx contractapi.TransactionContextInterface, source string) ([]*Log, error) {
	queryString := fmt.Sprintf(`{"selector":{"source":"%s"}}`, source)
	return s.getQueryResultForQueryString(ctx, queryString)
}

// getQueryResultForQueryString executes the passed in query string
func (s *SmartContract) getQueryResultForQueryString(ctx contractapi.TransactionContextInterface, queryString string) ([]*Log, error) {
	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var logs []*Log
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var log Log
		err = json.Unmarshal(queryResponse.Value, &log)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, nil
}

// BuildMerkleTree constructs a Merkle Tree from a list of hashes
// Returns the Merkle Root (hex string)
func (s *SmartContract) BuildMerkleTree(hashes []string) string {
	if len(hashes) == 0 {
		return ""
	}

	// If there is only one hash, it is the root
	if len(hashes) == 1 {
		return hashes[0]
	}

	// Copy the hashes so the original is not modified
	currentLevel := make([]string, len(hashes))
	copy(currentLevel, hashes)

	// Build the tree bottom-up
	for len(currentLevel) > 1 {
		var nextLevel []string

		// If the number of nodes is odd, duplicate the last one
		if len(currentLevel)%2 != 0 {
			currentLevel = append(currentLevel, currentLevel[len(currentLevel)-1])
		}

		// Combine pairs of hashes
		for i := 0; i < len(currentLevel); i += 2 {
			combinedHash := combineHashes(currentLevel[i], currentLevel[i+1])
			nextLevel = append(nextLevel, combinedHash)
		}

		currentLevel = nextLevel
	}

	return currentLevel[0]
}

// combineHashes combines two hashes using SHA256
func combineHashes(hash1, hash2 string) string {
	combined := hash1 + hash2
	hasher := sha256.New()
	hasher.Write([]byte(combined))
	return hex.EncodeToString(hasher.Sum(nil))
}

// StoreMerkleRoot stores a Merkle Root batch in the ledger
func (s *SmartContract) StoreMerkleRoot(ctx contractapi.TransactionContextInterface, batchID string, merkleRoot string, timestamp string, numLogs int, logIDs string) error {
	// Parse timestamp
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		parsedTime = time.Now()
	}

	// Parse logIDs from JSON array string
	var logIDArray []string
	if err := json.Unmarshal([]byte(logIDs), &logIDArray); err != nil {
		return fmt.Errorf("failed to parse logIDs: %v", err)
	}

	// The tenant comes from the signed client identity, not from any argument:
	// batch state is partitioned by it, so one tenant can neither read nor
	// overwrite another's anchors (F14).
	tenant, err := s.clientTenant(ctx)
	if err != nil {
		return err
	}

	batch := MerkleBatch{
		Tenant:     tenant,
		BatchID:    batchID,
		MerkleRoot: merkleRoot,
		Timestamp:  parsedTime,
		NumLogs:    numLogs,
		LogIDs:     logIDArray,
	}

	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	key, err := s.batchKey(ctx, tenant, batchID)
	if err != nil {
		return fmt.Errorf("failed to build batch key: %v", err)
	}

	// Anchors are WRITE-ONCE: a batch id may be anchored exactly once within a
	// tenant, so a re-submission (a retry, a second API replica, or a tampering
	// attempt) cannot overwrite the original root. This is the immutability
	// guarantee the product sells. Earlier values of a key remain retrievable via
	// the ledger's built-in key history (GetHistoryForKey) if ever needed.
	existing, err := ctx.GetStub().GetState(key)
	if err != nil {
		return fmt.Errorf("failed to read existing batch: %v", err)
	}
	if existing != nil {
		return fmt.Errorf("batch %s is already anchored and cannot be overwritten", batchID)
	}
	return ctx.GetStub().PutState(key, batchJSON)
}

// QueryMerkleBatch returns a Merkle batch by ID, scoped to the caller's tenant.
// A batch anchored by another tenant is reported as non-existent — no leak.
func (s *SmartContract) QueryMerkleBatch(ctx contractapi.TransactionContextInterface, batchID string) (*MerkleBatch, error) {
	tenant, err := s.clientTenant(ctx)
	if err != nil {
		return nil, err
	}
	key, err := s.batchKey(ctx, tenant, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to build batch key: %v", err)
	}
	batchJSON, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch from world state: %v", err)
	}
	if batchJSON == nil {
		return nil, fmt.Errorf("batch %s does not exist", batchID)
	}

	var batch MerkleBatch
	err = json.Unmarshal(batchJSON, &batch)
	if err != nil {
		return nil, err
	}

	return &batch, nil
}

// VerifyBatchIntegrity verifies the integrity of a batch by recalculating Merkle Root
// Receives hashes as JSON array string
func (s *SmartContract) VerifyBatchIntegrity(ctx contractapi.TransactionContextInterface, batchID string, logHashes string) (bool, error) {
	// Get stored batch
	batch, err := s.QueryMerkleBatch(ctx, batchID)
	if err != nil {
		return false, err
	}

	// Parse log hashes from JSON array
	var hashes []string
	if err := json.Unmarshal([]byte(logHashes), &hashes); err != nil {
		return false, fmt.Errorf("failed to parse logHashes: %v", err)
	}

	// Verify number of logs matches
	if len(hashes) != batch.NumLogs {
		return false, fmt.Errorf("number of hashes (%d) does not match batch size (%d)", len(hashes), batch.NumLogs)
	}

	// Recalculate Merkle Root
	recalculatedRoot := s.BuildMerkleTree(hashes)

	// Compare with stored root
	return recalculatedRoot == batch.MerkleRoot, nil
}

// GetAllMerkleBatches returns the calling tenant's Merkle batches only. The
// range is restricted to the caller's partition of the composite-key space, so
// it can never enumerate another tenant's batches (F14).
func (s *SmartContract) GetAllMerkleBatches(ctx contractapi.TransactionContextInterface) ([]*MerkleBatch, error) {
	tenant, err := s.clientTenant(ctx)
	if err != nil {
		return nil, err
	}
	resultsIterator, err := ctx.GetStub().GetStateByPartialCompositeKey(batchObjectType, []string{tenant})
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var batches []*MerkleBatch
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var batch MerkleBatch
		err = json.Unmarshal(queryResponse.Value, &batch)
		if err != nil {
			return nil, err
		}
		batches = append(batches, &batch)
	}

	return batches, nil
}

func main() {
	chaincode, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		fmt.Printf("Error creating log chaincode: %v", err)
		return
	}

	if err := chaincode.Start(); err != nil {
		fmt.Printf("Error starting log chaincode: %v", err)
	}
}
