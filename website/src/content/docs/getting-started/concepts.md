---
title: Core concepts
description: The vocabulary of AeternisLog — records, domains, batches, Merkle roots, anchors, tenants, and the WAL.
sidebar:
  order: 2
---

A small vocabulary covers the whole system.

## Record

The unit of data. A record has a client-chosen **`source`**, an arbitrary JSON
**`payload`**, and a server-assigned `id`, `timestamp`, and integrity **`hash`**.
Records are append-only; "deleting" one is a soft-delete that preserves the audit
trail.

```json
{ "source": "crm", "payload": { "party": "acme", "amount": 100 } }
```

Optionally, `hash_fields` restricts which payload keys feed the hash, so other
fields can change without breaking integrity.

## Domain

A namespace for records, chosen freely in the URL: `/api/v1/{domain}/records`. A
*log* is simply a record in the `logs` domain — there is no separate logs API.
Domains keep unrelated record streams (e.g. `audit`, `contracts`, `shipments`)
separate.

## Hash & `hash_version`

Each record's integrity hash uses scheme **v2**:

```
SHA-256( 0x00 ‖ lp(id) ‖ lp(timestamp) ‖ lp(source) ‖ lp(canonical(payload)) )
```

where `lp(x)` is a length-prefixed field (so content can't shift across
boundaries) and `0x00` domain-separates a leaf from an internal node. The scheme
is recorded per record as `hash_version`, so older batches still verify. See
[cryptographic design](/aeternis-log/security/cryptography/).

## Batch & Merkle root

Pending records in a domain are grouped into a **batch** and folded into a
**Merkle tree**. The tree's **root** is a single hash committing to every record
in the batch. Batching happens automatically (size/interval) or on demand via
`POST .../records/batch`.

## Anchor

The Merkle root is written **once** to Hyperledger Fabric — the **anchor**. It is
immutable: the chaincode rejects any attempt to overwrite an anchored batch. The
anchoring transaction returns a `tx_id`.

## Tenant

Records are isolated per **`(tenant, domain)`**. When authentication is enabled, a
caller's tenant is resolved from its API key; a key for tenant A cannot read
tenant B's records. Isolation is also enforced on-chain by the caller's signed
identity. See [multi-tenancy](/aeternis-log/guides/multi-tenancy/).

## WAL (write-ahead log)

A durable, append-only log in front of record creation. A record is `fsync`-ed to
the WAL before it is acknowledged, so an acknowledged write survives a crash;
recovery replays the log idempotently.
