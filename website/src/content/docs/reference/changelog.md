---
title: Changelog
description: Notable changes to AeternisLog.
sidebar:
  order: 2
---

A high-level log of notable changes. For the full commit history, see
[GitHub](https://github.com/RicardoMBregalda/aeternis-log).

## Production hardening

A structured hardening pass across the stack:

### Security & cryptography
- Hardened **v2 hashing** — length-prefixed fields, leaf/node domain separation,
  versioned per record.
- **CVE-2012-2459** closed — odd Merkle nodes are promoted, not duplicated.
- **Ledger-enforced tenant isolation** — chaincode keys batch state by the
  caller's signed identity; composite `(tenant, batchID)` keys.
- **Per-tenant gateway identities** in the API (identity pool, selected by tenant).
- **Locked-down public endpoint** — server-side channel resolution; no metadata
  leaks.

### Durability & correctness
- **Record-aware WAL** with idempotent crash recovery.
- **Race-free batch claiming** + anchor **reconciler**.
- **Write-once anchors** enforced on-chain.

### Operations
- **Versioned schema migrations** — assert-not-mutate boot, unique version index,
  Helm pre-upgrade Job.
- **Gateway-only Fabric transport** — the Docker-socket transport removed.
- **Scripted chaincode upgrades** at incremented sequence.
- **Honest Helm chart** — WAL PVC, replica gating, least-privilege identity,
  datastore guidance.
- **Swagger** regenerated in the image build; dead endpoints/config pruned.

### Tooling
- Reworked **web integrity dashboard** for the records API, served by nginx.
- This **documentation site**.

:::note
Versioned releases and tags will be listed here as they are published.
:::
