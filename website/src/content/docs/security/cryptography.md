---
title: Cryptographic design
description: The v2 hashing scheme, the Merkle tree construction, and the attacks they close.
sidebar:
  order: 2
---

## Record hashing (scheme v2)

Each record's integrity hash is:

```
SHA-256( 0x00 ‖ lp(id) ‖ lp(timestamp) ‖ lp(source) ‖ lp(canonical(payload)) )
```

- **`lp(x)`** = an 8-byte big-endian length prefix followed by `x`. Length
  prefixing prevents **field-shift**: without it, `source="ab", payload="c"` and
  `source="a", payload="bc"` could hash to the same value. With it, they cannot.
- **`0x00`** tags the input as a Merkle **leaf**. Internal nodes are tagged
  `0x01`. This **domain separation** ensures a leaf hash can never be mistaken for
  an internal-node hash.
- **`canonical(payload)`** is JSON with sorted keys, compact, HTML-escaped —
  matching Go's `encoding/json`. The hash is independent of key order and
  byte-identical across the server and both SDKs.
- **`hash_fields`** optionally restricts which payload keys feed the hash.
- **`hash_version`** records the scheme per record, so batches anchored under the
  older v1 scheme still verify after an upgrade.

## Merkle tree

Record hashes (the leaves) are folded pairwise into internal nodes until a single
**root** remains. Each internal node is `SHA-256(0x01 ‖ left ‖ right)`.

### Odd-node promotion (CVE-2012-2459)

When a level has an odd number of nodes, the trailing node is **promoted** to the
next level unchanged — it is **not duplicated**. Duplicating the last node (as
naive Merkle implementations do) lets an attacker construct two different trees
with the same root. Promotion closes that vulnerability (CVE-2012-2459).

## Why this matters

These choices make the core guarantee **structural**, not incidental:

- no two distinct record sets produce the same leaf (length prefixing);
- a leaf can't be passed off as a node (domain separation);
- no two distinct trees produce the same root (odd-node promotion).

Combined with the immutable on-chain anchor, altering anchored data is
mathematically detectable. See the
[tamper-evidence guarantee](/aeternis-log/security/tamper-evidence/).
