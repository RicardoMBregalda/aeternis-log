---
title: 'Use case: Supply chain'
description: Anchor chain-of-custody events so every party can verify provenance without trusting a central operator.
sidebar:
  order: 7
  label: 'Use case: Supply chain'
---

## The problem

A supply chain spans many organizations that don't fully trust each other —
suppliers, carriers, warehouses, retailers. Each records custody events in its own
systems. When a dispute arises ("was this batch refrigerated the whole way?",
"who handled it on the 4th?"), there's no shared, tamper-proof record everyone can
rely on.

## How AeternisLog helps

Anchor each custody event's fingerprint on a permissioned ledger that all parties
can verify against — without putting sensitive commercial data on-chain.

1. **Record** events under a domain such as `shipments` — handoffs, sensor
   readings, inspections — each as a JSON payload.
2. **Anchor** their Merkle roots on the blockchain, write-once and timestamped.
3. **Verify** independently: any party recomputes the root from the records it
   holds and checks it against the chain. A match proves the records are exactly
   what was anchored.

## Why it fits multi-party chains

- **No central trust.** Verification needs only the records and the on-chain root
  — not trust in whoever ran the API.
- **Selective sharing.** Only fingerprints are anchored; the underlying payloads
  stay private and are shared on a need-to-know basis.
- **Provenance over time.** Timestamped anchors build an ordered, tamper-evident
  history of custody.
- **Per-tenant isolation.** Each organization's records are isolated, with
  optional per-tenant ledgers (channels).

## A concrete flow

```bash
# A carrier records a refrigerated-transit reading
POST /api/v1/shipments/records  {"source":"reefer-04","payload":{"shipment":"S-2231","tempC":3.4,"at":"2026-06-22T09:00:00Z"}}

# Anchor the batch
POST /api/v1/shipments/records/batch

# A downstream party verifies the chain of custody it received
aeternislog verify --file custody.csv --api https://anchor.example.com --domain shipments --batch-id <batch_id>
```

The same independent proof underpins compliance, recalls, and dispute resolution.
