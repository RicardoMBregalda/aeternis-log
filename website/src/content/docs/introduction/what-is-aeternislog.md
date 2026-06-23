---
title: What is AeternisLog
description: AeternisLog gives any data cryptographic proof of integrity by anchoring Merkle roots on a permissioned Hyperledger Fabric blockchain.
sidebar:
  order: 1
---

**AeternisLog** is a platform that gives any data **cryptographic proof of
integrity**. You send records to a REST API; they are stored in MongoDB (fast and
queryable), grouped into **Merkle trees**, and the root of each batch is anchored
**once** on **Hyperledger Fabric** (immutable and auditable).

Any tampering is mathematically detectable: an auditor recomputes the Merkle root
from the stored records and compares it against the root stored on the
blockchain. They match only if nothing changed.

## What it is — and isn't

AeternisLog is the implementation of a single, focused pattern:

> **Tamper-Evident Data Anchoring** — keep data where it is fast to use, and
> anchor a cryptographic fingerprint of it on an independent, immutable ledger.

- **It is** an integrity layer: a way to prove that a set of records is complete
  and unaltered, independently of whoever stores them.
- **It is not** a database replacement, a general-purpose blockchain, or a place
  to put large payloads on-chain. Only compact Merkle roots are anchored; your
  data stays in MongoDB.

## What you get

| Component | Role |
|---|---|
| **REST API** (Go) | Create records, batch them into Merkle trees, anchor and verify. |
| **MongoDB** | Off-chain storage for records (the hot, queryable tier). |
| **Hyperledger Fabric** | Permissioned blockchain that stores the Merkle roots, write-once. |
| **Redis** | Optional cache and shared rate limiter (graceful degradation if absent). |
| **Go & Python SDKs** | Recompute hashes and Merkle roots locally for trustless verification. |
| **Web dashboard** | A dependency-free UI to create, anchor, and verify records. |

## Who it's for

- **Compliance and audit** teams who must prove records were not altered.
- **Platforms** that want to offer their customers independent integrity proofs.
- **Engineering teams** building supply-chain, financial, legal, or security-log
  systems where "trust me" is not good enough.

## Next steps

- [Why tamper-evidence matters](/aeternis-log/introduction/why-tamper-evidence/) — the problem AeternisLog solves.
- [How it works](/aeternis-log/introduction/how-it-works/) — the architecture and the trust flow.
- [Quickstart](/aeternis-log/getting-started/quickstart/) — run the stack and anchor your first record.
