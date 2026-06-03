package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/internal/wal"
	"github.com/gin-gonic/gin"
)

func setupWALTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestWALHandlerCreation(t *testing.T) {
	walInstance, _ := wal.NewWriteAheadLog("/tmp/test-wal", 5*time.Second)

	handler := NewWALHandler(walInstance)

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestGetWALStats(t *testing.T) {
	walInstance, _ := wal.NewWriteAheadLog("/tmp/test-wal", 5*time.Second)

	handler := NewWALHandler(walInstance)

	router := setupWALTestRouter()
	router.GET("/wal/stats", handler.GetStats)

	httpReq, _ := http.NewRequest("GET", "/wal/stats", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestGetWALHealth(t *testing.T) {
	walInstance, _ := wal.NewWriteAheadLog("/tmp/test-wal", 5*time.Second)

	handler := NewWALHandler(walInstance)

	router := setupWALTestRouter()
	router.GET("/wal/health", handler.GetHealth)

	httpReq, _ := http.NewRequest("GET", "/wal/health", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	// Should return 503 because processor is not running
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 200 or 503, got %d", w.Code)
	}
}

func TestForceProcessWithNilWAL(t *testing.T) {
	handler := NewWALHandler(nil)

	router := setupWALTestRouter()
	router.POST("/wal/force-process", handler.ForceProcess)

	httpReq, _ := http.NewRequest("POST", "/wal/force-process", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}
