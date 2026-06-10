// Package webhook delivers signed callbacks to an external URL when a batch is
// anchored on the blockchain, for integrations that react to new anchors.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/logger"
	"github.com/RicardoMBregalda/aeternis-log/go-api/pkg/config"
)

// BatchAnchoredEvent is the payload POSTed to the webhook when a batch is
// anchored on Fabric.
type BatchAnchoredEvent struct {
	Event      string `json:"event"` // always "batch.anchored"
	Tenant     string `json:"tenant"`
	Domain     string `json:"domain"`
	BatchID    string `json:"batch_id"`
	MerkleRoot string `json:"merkle_root"`
	NumRecords int    `json:"num_records"`
	TxID       string `json:"tx_id"`
	AnchoredAt string `json:"anchored_at"`
}

// Notifier delivers BatchAnchoredEvents to the configured webhook URL.
type Notifier struct {
	cfg    config.WebhookConfig
	client *http.Client
}

// New creates a Notifier from configuration.
func New(cfg config.WebhookConfig) *Notifier {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Notifier{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

// Enabled reports whether the webhook is configured and on.
func (n *Notifier) Enabled() bool {
	return n != nil && n.cfg.Enabled && n.cfg.URL != ""
}

// NotifyBatchAnchored delivers the event asynchronously (fire-and-forget with
// retries) so it never blocks or fails batch processing — the batch is already
// anchored on-chain regardless of the callback outcome.
func (n *Notifier) NotifyBatchAnchored(event BatchAnchoredEvent) {
	if !n.Enabled() {
		return
	}
	event.Event = "batch.anchored"
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	go n.deliver(body, event.BatchID)
}

// deliver POSTs the body, retrying up to MaxRetries with a linear backoff.
func (n *Notifier) deliver(body []byte, batchID string) {
	for attempt := 0; attempt <= n.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		if err := n.post(body); err == nil {
			return
		} else if attempt == n.cfg.MaxRetries {
			lg := logger.WithRequestID("")
			lg.Warn().Err(err).Str("batch_id", batchID).Int("attempts", attempt+1).Msg("webhook delivery failed")
		}
	}
}

// post sends a single signed POST and returns an error on transport failure or a
// non-2xx response.
func (n *Notifier) post(body []byte) error {
	req, err := http.NewRequest(http.MethodPost, n.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "batch.anchored")
	if n.cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(n.cfg.Secret))
		mac.Write(body)
		req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
