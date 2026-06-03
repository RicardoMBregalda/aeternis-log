package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/gin-gonic/gin"
)

func setupHealthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestHealthHandlerCreation(t *testing.T) {
	redisCfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(redisCfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewHealthHandler(nil, nil, cache, fabricClient, processor, "test", "test")
	
	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	redisCfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(redisCfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewHealthHandler(nil, nil, cache, fabricClient, processor, "test", "test")
	
	router := setupHealthTestRouter()
	router.GET("/health", handler.HealthCheck)
	
	httpReq, _ := http.NewRequest("GET", "/health", nil)
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)
	
	// Should return response even with failing services
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 200 or 503, got %d", w.Code)
	}
}

func TestGetStatsEndpoint(t *testing.T) {
	redisCfg := &config.RedisConfig{
		CacheEnabled: false,
	}
	cache, _ := cache.NewRedisClient(redisCfg)
	
	batchCfg := &config.BatchingConfig{
		Enabled:              false,
		BatchExecutorWorkers: 2,
	}
	fabricCfg := &config.FabricConfig{
		SyncEnabled: false,
	}
	fabricClient := fabric.NewFabricClient(fabricCfg)
	processor := merkle.NewBatchProcessor(nil, fabricClient, batchCfg)
	
	handler := NewHealthHandler(nil, nil, cache, fabricClient, processor, "test", "test")
	
	router := setupHealthTestRouter()
	router.GET("/stats", handler.GetStats)
	
	httpReq, _ := http.NewRequest("GET", "/stats", nil)
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)
	
	// Should return stats even without database
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHealthCheckWithNilComponents(t *testing.T) {
	// Test with minimal components
	handler := NewHealthHandler(nil, nil, nil, nil, nil, "test", "test")
	
	router := setupHealthTestRouter()
	router.GET("/health", handler.HealthCheck)
	
	httpReq, _ := http.NewRequest("GET", "/health", nil)
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)
	
	// Should handle gracefully even with nil components
	if w.Code != http.StatusServiceUnavailable {
		t.Logf("Got status %d (expected 503 with nil components)", w.Code)
	}
}
