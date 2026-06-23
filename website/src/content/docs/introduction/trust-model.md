---
title: Trust & security model
description: What AeternisLog guarantees, what it assumes, and where each guarantee is enforced.
sidebar:
  order: 4
---

AeternisLog is an **integrity** system. This page states precisely what it
guarantees, what it assumes, and where the boundaries are — so you can reason
about it honestly.

## What it guarantees

- **Tamper-evidence.** If an anchored record is altered, the recomputed Merkle
  root will not match the root on the chain. The alteration is detectable by
  anyone holding the data.
- **Immutability of anchors.** A Merkle root is anchored write-once. Re-submitting
  a batch id — a retry, a second replica, or an attacker — cannot overwrite the
  original root.
- **Independent verification.** Verification needs only the records and the
  on-chain root. The Go and Python SDKs recompute everything locally, so a client
  never has to trust the server's answer.
- **Point-in-time existence.** The anchor is timestamped, so it also proves the
  data existed in that exact form at that time.
- **Tenant isolation at the ledger.** Batch state is partitioned by the caller's
  signed identity; one tenant cannot read, overwrite, or enumerate another's
  anchors. See [tenant isolation](/aeternis-log/security/tenant-isolation/).

## What it assumes

- **The Fabric network is trustworthy for ordering and immutability.** As with any
  blockchain, the immutability guarantee rests on the permissioned network's
  consensus (Raft ordering, endorsement policy). A majority collusion of orderers
  is out of scope, as for any such system.
- **Keys are kept secret.** API keys and Fabric signing identities must be
  protected. AeternisLog supports hashing API keys at rest and least-privilege
  identity bundles, but key custody is the operator's responsibility.
- **The hash function is sound.** Integrity rests on SHA-256 collision resistance.

## What it does not do

- It does not prevent deletion of off-chain data — it makes deletion or alteration
  **detectable** (the anchor remains and no longer matches).
- It does not store your data on-chain — only Merkle roots are anchored.
- It is not a confidentiality system on its own; payload encryption is the
  caller's responsibility.

## Defense in depth

Beyond the core cryptography, the platform was hardened across the stack:
versioned hashing, a durable WAL, race-free batch claiming, a locked-down public
verification endpoint, a Redis-backed shared rate limiter, schema migrations
separated from boot, and ledger-enforced tenant isolation. See the
[hardening summary](/aeternis-log/security/hardening/) and the
[threat model](/aeternis-log/security/threat-model/).
