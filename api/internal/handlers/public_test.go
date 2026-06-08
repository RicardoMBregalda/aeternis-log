package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/fabric"
	"github.com/gin-gonic/gin"
)

type fakeQuerier struct {
	resp       *fabric.QueryResponse
	err        error
	gotChannel string
	gotBatch   string
}

func (f *fakeQuerier) VerifyMerkleBatch(_ context.Context, channel, batchID string) (*fabric.QueryResponse, error) {
	f.gotChannel = channel
	f.gotBatch = batchID
	return f.resp, f.err
}

func setupPublic(q batchQuerier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/public/anchors/:batchId", NewPublicHandler(q, "logchannel").GetAnchor)
	return r
}

func do(r *gin.Engine, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestGetAnchorFoundUsesDefaultChannel(t *testing.T) {
	q := &fakeQuerier{resp: &fabric.QueryResponse{Data: map[string]interface{}{"batch_id": "b1", "merkle_root": "root123"}}}
	w := do(setupPublic(q), "/public/anchors/b1")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"anchored":true`) {
		t.Errorf("expected anchored true, got %s", w.Body.String())
	}
	if q.gotChannel != "logchannel" || q.gotBatch != "b1" {
		t.Errorf("querier got channel=%q batch=%q", q.gotChannel, q.gotBatch)
	}
}

func TestGetAnchorChannelOverrideAndRootMatch(t *testing.T) {
	q := &fakeQuerier{resp: &fabric.QueryResponse{Data: map[string]interface{}{"merkle_root": "root123"}}}
	w := do(setupPublic(q), "/public/anchors/b1?channel=acme-channel&root=root123")
	if q.gotChannel != "acme-channel" {
		t.Errorf("expected channel override, got %q", q.gotChannel)
	}
	if !strings.Contains(w.Body.String(), `"root_matches":true`) {
		t.Errorf("expected root_matches true, got %s", w.Body.String())
	}
}

func TestGetAnchorRootMismatch(t *testing.T) {
	q := &fakeQuerier{resp: &fabric.QueryResponse{Data: map[string]interface{}{"merkle_root": "root123"}}}
	w := do(setupPublic(q), "/public/anchors/b1?root=WRONG")
	if !strings.Contains(w.Body.String(), `"root_matches":false`) {
		t.Errorf("expected root_matches false, got %s", w.Body.String())
	}
}

func TestGetAnchorNotFound(t *testing.T) {
	q := &fakeQuerier{err: errors.New("batch b1 does not exist")}
	w := do(setupPublic(q), "/public/anchors/b1")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetAnchorFabricError(t *testing.T) {
	q := &fakeQuerier{err: errors.New("connection refused")}
	w := do(setupPublic(q), "/public/anchors/b1")
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}
