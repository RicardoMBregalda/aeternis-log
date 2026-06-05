package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RicardoMBregalda/tcc-log-management/go-api/pkg/config"
)

func newTestNotifier(url, secret string, retries int) *Notifier {
	return New(config.WebhookConfig{Enabled: true, URL: url, Secret: secret, Timeout: 2 * time.Second, MaxRetries: retries})
}

// TestNotifierDelivers verifies the async delivery posts a signed batch.anchored event.
func TestNotifierDelivers(t *testing.T) {
	received := make(chan []byte, 1)
	sig := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig <- r.Header.Get("X-Webhook-Signature")
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secret := "topsecret"
	n := newTestNotifier(srv.URL, secret, 0)
	n.NotifyBatchAnchored(BatchAnchoredEvent{Domain: "audit", BatchID: "b1", MerkleRoot: "root", NumRecords: 3, TxID: "tx"})

	select {
	case body := <-received:
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if got := <-sig; got != want {
			t.Errorf("signature: got %q want %q", got, want)
		}
		var ev BatchAnchoredEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.Event != "batch.anchored" || ev.BatchID != "b1" || ev.Domain != "audit" {
			t.Errorf("unexpected event payload: %+v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("webhook was not delivered")
	}
}

// TestNotifierRetries verifies failed deliveries are retried up to MaxRetries.
func TestNotifierRetries(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&count, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := newTestNotifier(srv.URL, "", 3)
	n.deliver([]byte(`{"event":"batch.anchored"}`), "b1")

	if c := atomic.LoadInt32(&count); c != 3 {
		t.Errorf("expected 3 attempts (2 failures + 1 success), got %d", c)
	}
}

// TestNotifierDisabled verifies a disabled notifier sends nothing.
func TestNotifierDisabled(t *testing.T) {
	var hit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hit, 1)
	}))
	defer srv.Close()

	n := New(config.WebhookConfig{Enabled: false, URL: srv.URL})
	n.NotifyBatchAnchored(BatchAnchoredEvent{BatchID: "b1"})

	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt32(&hit) != 0 {
		t.Error("disabled notifier must not send any request")
	}
}
