---
title: FAQ
description: Common questions about AeternisLog — how it compares to alternatives, what it stores on-chain, and how verification works.
sidebar:
  order: 3
---

## How is this different from just hashing my logs?

A local hash proves nothing if the same party can recompute and replace it.
AeternisLog anchors the hash (a Merkle root) on an **independent, immutable**
ledger, so the proof doesn't depend on trusting whoever stores the data.

## How is it different from "putting data on a blockchain"?

Only compact **Merkle roots** go on-chain — never your data. Your records stay in
MongoDB where they're fast, queryable, and private. You get blockchain-grade
immutability for the proof and database-grade performance for the data.

## What is actually stored on-chain?

A batch's Merkle root, a batch id, a record count, a timestamp, and (for the
caller's own records) the record-id list — nothing else. Payloads never leave your
database.

## Do I have to trust AeternisLog to believe a verification?

No. The Go and Python SDKs (and the offline CLI) recompute the hashes and Merkle
root **locally** and compare them with the root read from the blockchain. The
server is never in the trust path for verification.

## What blockchain does it use?

A permissioned **Hyperledger Fabric** network (Raft ordering). It's private and
permissioned — no public chain, no gas, no cryptocurrency.

## Can one tenant see another tenant's data?

No. Records are isolated per `(tenant, domain)` at the API, and — with per-tenant
identities — isolation is enforced **at the ledger** by the caller's signed
identity. See [tenant isolation](/aeternis-log/security/tenant-isolation/).

## What happens if anchoring fails?

Records are marked `anchor_failed` (never lost) and a reconciler re-drives them.
The WAL guarantees an acknowledged record survives a crash.

## Is my data encrypted?

AeternisLog guarantees **integrity**, not confidentiality. Encrypt sensitive
payload fields before sending if you need confidentiality; the integrity proof
works over whatever bytes you store.

## Can I run it in production today?

Yes — the platform is hardened (see the [hardening summary](/aeternis-log/security/hardening/)).
Review the [hardening checklist](/aeternis-log/operations/hardening-checklist/) and
the known limitations in [tenant isolation](/aeternis-log/security/tenant-isolation/)
for multi-tenant deployments.

## How do I get support?

Community support is via [GitHub](https://github.com/RicardoMBregalda/aeternis-log/issues).
For SLAs and assisted deployments, [open an issue](https://github.com/RicardoMBregalda/aeternis-log/issues/new).
