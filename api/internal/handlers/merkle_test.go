package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/models"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/gin-gonic/gin"
)

func setupMerkleTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestMerkleHandlerCreation(t *testing.T) {
	cfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(cfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewMerkleHandler(processor, cache)
	
	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestCreateBatchRequest(t *testing.T) {
	cfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(cfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewMerkleHandler(processor, cache)
	
	router := setupMerkleTestRouter()
	router.POST("/merkle/batch", handler.CreateBatch)
	
	// Test with valid request
	req := models.CreateBatchRequest{
		BatchSize: 50,
	}
	
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", "/merkle/batch", bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)
	
	// Should fail because processor is not started, but validates request
	// In real scenario, we'd start the processor first
}

func TestVerifyBatchValidation(t *testing.T) {
	cfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(cfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewMerkleHandler(processor, cache)
	
	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
	
	// Note: Full test would require mock database
}

func TestListBatchesEndpoint(t *testing.T) {
	cfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(cfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewMerkleHandler(processor, cache)
	
	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
	
	// Note: Full test would require mock database
}

func TestGetBatchStats(t *testing.T) {
	cfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(cfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewMerkleHandler(processor, cache)
	
	router := setupMerkleTestRouter()
	router.GET("/merkle/stats", handler.GetBatchStats)
	
	httpReq, _ := http.NewRequest("GET", "/merkle/stats", nil)
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)
	
	// Should return stats even without database
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}
}
