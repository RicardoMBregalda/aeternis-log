---
title: Create & anchor records
description: How records flow from creation to a Merkle batch anchored on the blockchain — automatically or on demand.
sidebar:
  order: 1
---

## Creating records

A record is a JSON `payload` under a domain, with a `source`:

```bash
curl -s -X POST $BASE/api/v1/audit/records -H 'Content-Type: application/json' -d '{
  "source": "payments",
  "payload": { "event": "charge", "amount": 1200 }
}'
```

The API computes the integrity hash and stores the record (after a durable WAL
write). Records are **append-only** — a `DELETE` is a soft-delete that preserves
the audit trail.

### Controlling the hash

By default the whole payload feeds the hash. Use `hash_fields` to pin integrity to
specific keys, so unrelated fields can change without breaking it:

```json
{ "source": "crm", "payload": { "party": "acme", "amount": 100, "note": "draft" },
  "hash_fields": ["party", "amount"] }
```

## Anchoring

Pending records are folded into a Merkle tree and the root is anchored on Fabric.

### On demand

```bash
curl -s -X POST $BASE/api/v1/audit/records/batch
# -> { "batch_id": "...", "merkle_root": "...", "tx_id": "...", "anchored": true }
```

### Automatically

Enable auto-batching so the processor anchors on a size or time trigger:

```yaml
batching:
  enabled: true
  auto_batch_size: 100
  auto_batch_interval: 30s
```

## Durability & correctness

- **Zero loss:** a record is `fsync`-ed to the WAL before it is acknowledged; a
  crash is recovered by idempotent replay.
- **No double-anchoring:** batch claiming is atomic, so concurrent batchers (or
  replicas) never anchor the same record twice.
- **Self-healing:** if anchoring fails, records are marked `anchor_failed` and a
  reconciler re-drives them — never silently lost.

Next: [verify integrity](/aeternis-log/guides/verify-integrity/).
