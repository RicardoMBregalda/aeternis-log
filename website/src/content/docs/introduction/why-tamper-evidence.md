---
title: Why tamper-evidence
description: Most systems can be quietly rewritten by whoever controls the database. Tamper-evident anchoring makes alteration mathematically detectable.
sidebar:
  order: 2
---

## The problem

In most systems, the party that stores the data can also change it. Audit logs,
financial records, and compliance trails are usually just rows in a database —
and rows can be updated or deleted by an administrator, a compromised service, or
a malicious insider, often without a trace.

"Trust us, we didn't change it" is not a proof. For regulated, contractual, or
security-sensitive data, you need a guarantee that **does not depend on trusting
the operator**.

## Why not just use a blockchain for everything?

Putting raw data on a public blockchain is slow, expensive, leaks confidential
information, and still doesn't make the data queryable. It is the wrong tool for
storage.

AeternisLog takes the opposite approach: keep the data **off-chain** where it is
fast and private, and anchor only a tiny, irreversible **fingerprint** (a Merkle
root) on a permissioned chain. You get blockchain-grade immutability for the
proof, and database-grade performance for the data.

## What "tamper-evident" buys you

- **Detection, not just prevention.** Even if someone alters a stored record, the
  recomputed Merkle root no longer matches the anchored one — the change is
  exposed.
- **Independent verification.** Anyone holding the records can recompute the root
  and check it against the chain. No need to trust AeternisLog, the database, or
  the operator.
- **A point-in-time proof.** The anchor is timestamped and write-once, so it also
  proves the data existed in that exact form at that time.

## How AeternisLog delivers it

1. Each record gets a content hash using a hardened scheme (length-prefixed
   fields, domain separation, versioned).
2. Records are folded into a **Merkle tree**; its root summarizes the whole batch.
3. The root is anchored **once** on Hyperledger Fabric — it cannot be overwritten.
4. Verification recomputes the root from current data and compares it to the
   chain, returning `VALID` or `CORRUPTED`.

Continue to [How it works](/aeternis-log/introduction/how-it-works/) for the full
architecture, or read the [trust & security model](/aeternis-log/introduction/trust-model/)
for exactly what is and isn't guaranteed.
