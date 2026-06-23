---
title: Quickstart
description: Bring up the full AeternisLog stack locally and anchor & verify your first record in minutes.
sidebar:
  order: 1
---

This brings up the full stack — a Hyperledger Fabric network plus the API
(Go + MongoDB + Redis) — and runs the whole tamper-evident flow: create a record,
anchor it on the blockchain, and verify its integrity.

## Prerequisites

- Docker + Docker Compose
- `make`
- (optional) Go 1.21+ / Python 3.8+ to use the [SDKs](/aeternis-log/sdks/go/)

## 1. Bring up the stack

```bash
make up        # Fabric network + API + MongoDB + Redis + dashboard
make status    # list the running containers
```

When it finishes:

| Service | URL |
|---|---|
| API | http://localhost:5001 |
| Health | http://localhost:5001/health |
| Swagger | http://localhost:5001/swagger/index.html |
| Metrics | http://localhost:9090/metrics |
| Dashboard | http://localhost:8088 |

## 2. Create a record

A record is any JSON `payload` under a client-chosen **domain**. The API computes
an integrity hash automatically.

```bash
curl -s -X POST http://localhost:5001/api/v1/audit/records \
  -H 'Content-Type: application/json' \
  -d '{"source":"payments","payload":{"event":"charge","amount":1200}}'
```

```json
{ "status": "success", "data": { "domain": "audit", "id": "…", "hash": "…" } }
```

## 3. Anchor the batch

Group the domain's pending records into a Merkle tree and anchor the root on the
blockchain.

```bash
curl -s -X POST http://localhost:5001/api/v1/audit/records/batch
```

```json
{
  "batch_id": "default-audit-3290b38d",
  "merkle_root": "20e1a987…",
  "num_records": 1,
  "tx_id": "1d96e697…",
  "anchored": true,
  "channel": "logchannel"
}
```

## 4. Verify integrity

```bash
curl -s -X POST http://localhost:5001/api/v1/audit/records/verify/default-audit-3290b38d
```

```json
{ "is_valid": true, "integrity": "VALID", "anchor_status": "ANCHORED",
  "on_chain_merkle_root": "20e1a987…", "recalculated_merkle_root": "20e1a987…" }
```

If any record had been altered, `integrity` would be `CORRUPTED` and the
recomputed root would differ from the anchored one.

## 5. Verify it yourself (trustless)

You don't have to trust the API's answer. The [Go](/aeternis-log/sdks/go/) and
[Python](/aeternis-log/sdks/python/) SDKs recompute the hashes and Merkle root
locally and compare them with the root read from the blockchain.

## Next steps

- [Core concepts](/aeternis-log/getting-started/concepts/) — records, domains, batches, anchors, tenants.
- [The integrity dashboard](/aeternis-log/getting-started/dashboard/) — do all of the above in a UI.
- [API reference](/aeternis-log/api/overview/) — every endpoint.

## Tear down

```bash
make down    # stop, keep data
make clean   # stop and remove volumes
```
