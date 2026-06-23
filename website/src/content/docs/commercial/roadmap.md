---
title: Roadmap
description: Where AeternisLog is heading — a high-level, non-binding view.
sidebar:
  order: 4
---

A high-level, non-binding view of where AeternisLog is heading. Priorities are
shaped by users and customers — [open a discussion](https://github.com/RicardoMBregalda/aeternis-log/discussions)
to weigh in.

## Recently shipped

- ✅ Hardened v2 hashing (length-prefixed, domain-separated, CVE-2012-2459 safe).
- ✅ Record-aware durable WAL with idempotent recovery.
- ✅ Race-free batch claiming + anchor reconciler.
- ✅ Ledger-enforced per-tenant isolation (identity-scoped chaincode keys).
- ✅ Per-tenant gateway identities in the API.
- ✅ Versioned schema migrations (assert-not-mutate boot) + Helm migration Job.
- ✅ Gateway-only Fabric transport (Docker-socket surface removed).

## In progress / next

- **Tenant-scoped public proofs** — a signed batch token so the unauthenticated
  public endpoint can verify per-tenant batches.
- **Backward-compatible chaincode upgrade** — a re-keying migration for batches
  anchored under the pre-isolation key scheme.
- **Per-record Merkle proofs** over the API (prove one record without the whole
  batch).

## Exploring

- Managed/hosted offering.
- Additional language SDKs.
- Pluggable anchor backends beyond Fabric.
- Retention & archival tiers for off-chain records.

:::note
This roadmap is indicative, not a commitment. Enterprise customers can influence
prioritization — see [editions](/aeternis-log/commercial/editions/).
:::
