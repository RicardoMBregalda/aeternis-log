---
title: Webhooks
description: Receive a signed callback whenever a batch is anchored on the blockchain.
sidebar:
  order: 4
---

Get notified the moment a batch is anchored, so downstream systems can react
without polling.

## Enable

```yaml
webhook:
  enabled: true
  url: "https://your-app.example.com/hooks/aeternislog"
  secret: "a-strong-shared-secret"   # enables HMAC signing
  timeout: 5s
  max_retries: 3
```

Or via environment: `WEBHOOK_ENABLED`, `WEBHOOK_URL`, `WEBHOOK_SECRET`.

## The `batch.anchored` event

Every time a batch is anchored, AeternisLog POSTs:

```json
{
  "event": "batch.anchored",
  "domain": "audit",
  "batch_id": "default-audit-3290b38d",
  "merkle_root": "20e1a987…",
  "num_records": 3,
  "tx_id": "1d96e697…",
  "anchored_at": "2026-06-22T22:51:05Z"
}
```

## Verifying the signature

When `secret` is set, the body is signed with **HMAC-SHA256** in the
`X-Webhook-Signature` header. Recompute it over the raw body and compare:

```python
import hmac, hashlib
expected = hmac.new(secret.encode(), raw_body, hashlib.sha256).hexdigest()
assert hmac.compare_digest(expected, request.headers["X-Webhook-Signature"])
```

## Delivery

Delivery is asynchronous with retries (up to `max_retries`), so a slow or briefly
unavailable receiver doesn't block anchoring. Make your handler **idempotent** —
treat a repeated `batch_id` as a no-op.
