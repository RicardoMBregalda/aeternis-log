---
title: Glossary
description: Key terms used across the AeternisLog documentation.
sidebar:
  order: 1
---

**Anchor** — a Merkle root written **once** to the blockchain. Immutable; the
chaincode rejects any overwrite.

**Batch** — a group of pending records folded into a Merkle tree and anchored
together. Identified by a `batch_id`.

**Chaincode** — the Go smart contract on Hyperledger Fabric that stores and guards
the anchors.

**Domain** — a client-chosen namespace for records (`/api/v1/{domain}/records`).

**Hash (v2)** — a record's integrity fingerprint:
`SHA-256(0x00 ‖ lp(id) ‖ lp(timestamp) ‖ lp(source) ‖ lp(canonical(payload)))`.

**`hash_fields`** — optional list restricting which payload keys feed the hash.

**`hash_version`** — the hashing scheme used for a record (v2; absent = legacy v1).

**Merkle root** — the single hash at the top of a Merkle tree; it commits to every
record in the batch.

**Merkle tree** — a binary tree of hashes; leaves are record hashes, internal nodes
are `SHA-256(0x01 ‖ left ‖ right)`. Odd nodes are **promoted**, not duplicated.

**MSP** — Membership Service Provider; a Fabric organization's identity authority.
Used as a tenant fallback when no `tenant` certificate attribute is present.

**Record** — the unit of data: `source` + JSON `payload`, with a server-assigned
`id`, `timestamp`, and `hash`.

**Tenant** — an isolation boundary. Records are isolated per `(tenant, domain)`;
ledger isolation is enforced by the caller's signed identity.

**WAL** — write-ahead log; records are `fsync`-ed to it before acknowledgement, so
an acknowledged write survives a crash.

**Write-once** — the property that an anchored Merkle root can never be overwritten.
