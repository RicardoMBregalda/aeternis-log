---
title: How it works
description: The AeternisLog architecture — records to MongoDB, Merkle batches, anchored on Hyperledger Fabric, and the trust flow that detects tampering.
sidebar:
  order: 3
---

## The flow

```
POST /api/v1/{domain}/records ─▶ MongoDB ─▶ batch (Merkle root) ─▶ anchor on Fabric
                                                                     │
            auditor / SDK recomputes the root ◀───────────────────────┘
                          compare with the anchored root → VALID or CORRUPTED
```

1. **Create.** A client POSTs a record (any JSON payload) to a domain. The API
   computes a content hash and stores the record in MongoDB.
2. **Batch.** Pending records in a domain are grouped (automatically, or on
   demand) and folded into a **Merkle tree**. The tree's **root** is a single hash
   that summarizes the whole batch.
3. **Anchor.** The API submits the Merkle root to Hyperledger Fabric, where it is
   written **once** under a key it can never overwrite. The transaction returns a
   `tx_id`.
4. **Verify.** To check integrity, the API (or your SDK) recomputes the Merkle
   root from the current records and compares it with the anchored root. A
   mismatch means something was altered.

## Components

| Component | Responsibility |
|---|---|
| **Go API** (Gin) | REST surface, hashing, Merkle batching, anchoring, verification, auth, rate limiting, metrics. |
| **MongoDB** | Off-chain storage for records (the queryable hot tier). |
| **Redis** | Optional cache and shared rate limiter; the API degrades gracefully without it. |
| **Hyperledger Fabric** | Permissioned ledger (Raft ordering) storing Merkle roots; chaincode in Go. |
| **WAL** | A durable write-ahead log in front of record creation, so an acknowledged write is never lost. |

## Durability: the write-ahead log

Before a record is acknowledged, it is appended and `fsync`-ed to a write-ahead
log, then inserted into MongoDB. If the process crashes between the two, recovery
replays the log idempotently — an acknowledged record is never lost.

## Why a Merkle tree?

Anchoring every record individually would be slow and expensive. A Merkle tree
lets a single root commit to thousands of records at once, while still allowing
**per-record** proofs. AeternisLog's tree uses domain-separated, length-prefixed
hashing and promotes odd nodes instead of duplicating them — closing a class of
Merkle attacks (CVE-2012-2459). See [cryptographic design](/aeternis-log/security/cryptography/).

## Where guarantees are enforced

- **Immutability** is enforced **on-chain**: the chaincode rejects any attempt to
  overwrite an anchored root.
- **Tenant isolation** is enforced **at the ledger boundary**: the chaincode keys
  batch state by the caller's signed identity, not by an argument.
- **Durability** is enforced **before acknowledgement**: the WAL fsyncs first.

Read the [trust & security model](/aeternis-log/introduction/trust-model/) for the
precise guarantees and their boundaries.
