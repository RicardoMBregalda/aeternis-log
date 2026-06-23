---
title: Threat model
description: Who AeternisLog defends against, what it assumes, and what is out of scope.
sidebar:
  order: 4
---

## Adversaries considered

| Adversary | Defense |
|---|---|
| **Insider / DB admin** alters a stored record | Detected: the recomputed Merkle root no longer matches the immutable anchor. |
| **Compromised API replica** tries to overwrite an anchor | Rejected on-chain: anchors are write-once. |
| **Malicious tenant** tries to read/forge another tenant's batch | Blocked at the ledger: batch state is keyed by the caller's signed identity. |
| **Network attacker** replays or tampers with a webhook | HMAC-SHA256 signature on the callback body. |
| **Unauthenticated prober** on the public endpoint | Server-side channel resolution + only "anchored yes/no + root" exposed; no tenant metadata. |
| **Brute-force / abuse** of the API | Opt-in API keys (hashable at rest) + a Redis-backed shared rate limiter. |
| **Oversized-body DoS** | Configurable request body cap (`413`). |

## Assumptions (trust boundaries)

- **The permissioned Fabric network is honest for ordering and immutability.** As
  with any blockchain, the guarantee rests on consensus (Raft ordering,
  endorsement policy). A majority collusion of orderers is out of scope.
- **Keys and identities are protected by the operator.** API keys and Fabric
  signing identities must be kept secret; the platform supports hashing keys at
  rest and least-privilege identity bundles, but custody is the operator's job.
- **SHA-256 is collision-resistant.**

## Out of scope

- **Confidentiality of payloads** — encrypt sensitive fields before sending.
- **Availability/backup of off-chain data** — deletion is *detectable*, not
  *prevented*.
- **Pre-anchor tampering** — integrity is guaranteed from the moment of anchoring;
  anchor promptly to shrink the window.
- **Physical/host compromise of the ledger nodes** — a standard blockchain-network
  assumption.

See the [hardening summary](/aeternis-log/security/hardening/) for the concrete
mitigations shipped across the stack.
