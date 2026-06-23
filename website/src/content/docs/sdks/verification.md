---
title: Local verification
description: The shared hashing and Merkle scheme that lets the Go server, Go SDK, and Python SDK produce byte-identical proofs.
sidebar:
  order: 3
---

The whole point of the SDKs is **trustless** verification: an auditor recomputes
the proof and compares it with the root anchored on the blockchain, never trusting
the API's answer.

## The shared scheme (v2)

Every implementation — the Go server, the Go SDK, and the Python SDK — computes a
record's hash identically:

```
SHA-256( 0x00 ‖ lp(id) ‖ lp(timestamp) ‖ lp(source) ‖ lp(canonical(payload)) )
```

- `lp(x)` is an 8-byte big-endian length followed by `x`, so content cannot shift
  across field boundaries undetected.
- `0x00` tags a Merkle **leaf**; internal nodes are tagged `0x01` — domain
  separation that closes a class of Merkle attacks (CVE-2012-2459).
- `canonical(payload)` matches Go's `encoding/json` (sorted keys, compact,
  HTML-escaped), so hashes are byte-identical across languages.
- `hash_fields`, when set, restricts which payload keys feed the hash.
- `hash_version` selects the scheme (absent = legacy v1), so old batches still
  verify after an upgrade.

:::caution[Cross-language number parity]
Payload numbers are decoded as 64-bit floats server-side, so integer-valued
numbers canonicalize without a decimal point (`7`, not `7.0`). For guaranteed
parity across languages, prefer integers and strings in payloads.
:::

## The verification step

1. Recompute each record's hash with the scheme above.
2. Fold the ordered hashes into a Merkle tree (odd nodes are **promoted**, not
   duplicated).
3. Compare the recomputed root with the root read from Fabric (or returned by the
   `/public/anchors/{batchId}` endpoint).

A match proves the records are exactly what was anchored. A mismatch proves
something changed. See [cryptographic design](/aeternis-log/security/cryptography/)
for the full rationale.
