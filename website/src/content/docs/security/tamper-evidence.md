---
title: Tamper-evidence guarantee
description: The precise guarantee AeternisLog makes about detecting alteration of anchored data.
sidebar:
  order: 1
---

## The guarantee

> Once a batch is anchored, any alteration of its records is **detectable** by
> anyone holding the records: the recomputed Merkle root will no longer equal the
> root stored on the blockchain.

This is *tamper-evidence*, not tamper-*prevention*. AeternisLog does not stop
someone with database access from changing a stored record — it makes the change
**impossible to hide**.

## Why it holds

- **The anchor is immutable.** The Merkle root is written once to a permissioned
  ledger; the chaincode rejects any overwrite. There is no "edit the anchor" path.
- **The hash is collision-resistant.** Producing a different set of records with
  the same Merkle root would require breaking SHA-256.
- **Hashing is unambiguous.** Length-prefixed fields and leaf/node domain
  separation mean no two distinct inputs collide by construction (no field-shift
  or second-preimage tricks). See [cryptographic design](/aeternis-log/security/cryptography/).

## What it also proves

Because the anchor is timestamped and write-once, a valid proof also establishes
**point-in-time existence**: the records existed in exactly that form at the time
of anchoring.

## What it does not cover

- **Availability of off-chain data.** If records are deleted, the anchor remains
  and no longer matches — the loss is detectable, but AeternisLog is not a backup.
- **Confidentiality.** Payloads are stored as given; encrypt sensitive fields
  before sending if needed.
- **Pre-anchor tampering.** Integrity is guaranteed from the moment of anchoring
  onward. Anchor promptly (auto-batching) to shrink the unanchored window.

See the [threat model](/aeternis-log/security/threat-model/) for assumptions and
boundaries.
