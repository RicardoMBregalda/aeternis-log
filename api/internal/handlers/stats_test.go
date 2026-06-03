package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/cache"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/fabric"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/merkle"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/wal"
	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
	"github.com/gin-gonic/gin"
)

func setupStatsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestStatsHandlerCreation(t *testing.T) {
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

	walInstance, _ := wal.NewWriteAheadLog("/tmp/test-wal", 5*time.Second)

	handler := NewStatsHandler(nil, nil, cache, fabricClient, processor, walInstance)

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestGetStats(t *testing.T) {
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

	walInstance, _ := wal.NewWriteAheadLog("/tmp/test-wal", 5*time.Second)

	handler := NewStatsHandler(nil, nil, cache, fabricClient, processor, walInstance)

	router := setupStatsTestRouter()
	router.GET("/stats", handler.GetStats)

	httpReq, _ := http.NewRequest("GET", "/stats", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestGetLogStats(t *testing.T) {
	handler := NewStatsHandler(nil, nil, nil, nil, nil, nil)

	router := setupStatsTestRouter()
	router.GET("/stats/logs", handler.GetLogStats)

	httpReq, _ := http.NewRequest("GET", "/stats/logs", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	// Should return 503 because collections is nil
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestGetSyncStats(t *testing.T) {
	handler := NewStatsHandler(nil, nil, nil, nil, nil, nil)

	router := setupStatsTestRouter()
	router.GET("/stats/sync", handler.GetSyncStats)

	httpReq, _ := http.NewRequest("GET", "/stats/sync", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	// Should return 503 because collections is nil
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}
