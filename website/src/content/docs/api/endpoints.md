---
title: Endpoints
description: Request and response shapes for every AeternisLog endpoint — create, list, batch, verify, report, health, and public anchor.
sidebar:
  order: 3
---

All examples assume `BASE=http://localhost:5001` and the `audit` domain.

## Create a record

```bash
POST /api/v1/{domain}/records
```

```bash
curl -s -X POST $BASE/api/v1/audit/records -H 'Content-Type: application/json' -d '{
  "source": "crm",
  "payload": { "party": "acme", "amount": 100 },
  "hash_fields": ["party", "amount"]
}'
```

```json
{ "status": "success", "message": "Record created successfully",
  "data": { "domain": "audit", "id": "22723eb2-…", "hash": "32818c68…" } }
```

`hash_fields` is optional — when present, only those payload keys feed the hash.

## List records

```bash
GET /api/v1/{domain}/records?limit=25&source=crm
```

```json
{ "records": [
  { "tenant": "default", "domain": "audit", "id": "58df…", "source": "crm",
    "payload": { "amount": 75, "party": "initech" }, "hash": "9cbd…",
    "hash_version": 2, "created_at": "2026-06-22T22:51:05Z" }
] }
```

## Get / delete a record

```bash
GET    /api/v1/{domain}/records/{id}
DELETE /api/v1/{domain}/records/{id}   # soft-delete; audit trail preserved
```

## Anchor a batch

```bash
POST /api/v1/{domain}/records/batch
```

```json
{ "batch_id": "default-audit-3290b38d", "tenant": "default", "domain": "audit",
  "merkle_root": "20e1a987…", "num_records": 3, "tx_id": "1d96e697…",
  "anchored": true, "channel": "logchannel" }
```

## Verify a batch

```bash
POST /api/v1/{domain}/records/verify/{batchId}
```

```json
{ "batch_id": "default-audit-3290b38d", "is_valid": true, "num_logs": 3,
  "original_merkle_root": "20e1a987…", "recalculated_merkle_root": "20e1a987…",
  "on_chain_merkle_root": "20e1a987…", "anchor_status": "ANCHORED",
  "integrity": "VALID", "message": "Batch integrity verified against the on-chain anchor" }
```

`integrity` is `VALID` or `CORRUPTED`. A corrupted batch (any record altered)
returns `is_valid: false` — it does not error.

## Audit report

```bash
GET /api/v1/{domain}/report          # JSON
GET /api/v1/{domain}/report.pdf      # PDF download
```

```json
{ "tenant": "default", "domain": "audit", "generated_at": "2026-06-22T22:51:06Z",
  "batches": [ { "batch_id": "default-audit-3290b38d", "merkle_root": "20e1a987…",
    "tx_id": "1d96e697…", "num_records": 3, "batched_at": "2026-06-22T22:51:05Z" } ],
  "total_batches": 1, "total_records": 3 }
```

## Health

```bash
GET /health
```

```json
{ "status": "healthy", "services": { "mongodb": "healthy", "redis": "healthy",
  "fabric": "healthy", "batch_processor": "running" } }
```

## Public anchor proof

Unauthenticated. Returns only "anchored: yes/no + timestamp + root" — never tenant
metadata. Optionally pass `?root=` to check a root you hold.

```bash
GET /public/anchors/{batchId}
```

```json
{ "anchored": true, "anchored_at": "2026-06-22T22:51:05Z",
  "batch_id": "default-audit-3290b38d", "merkle_root": "20e1a987…" }
```

A batch that isn't anchored returns `404`. See [verifying integrity](/aeternis-log/guides/verify-integrity/).
