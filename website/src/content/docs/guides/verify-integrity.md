---
title: Verify integrity
description: Three ways to verify a batch — server-side, trustless local recomputation, and the unauthenticated public proof.
sidebar:
  order: 2
---

There are three ways to verify a batch, from most convenient to most independent.

## 1. Server-side verify

The API recomputes the Merkle root from the current records and compares it with
the anchored root:

```bash
curl -s -X POST $BASE/api/v1/audit/records/verify/<batch_id>
```

```json
{ "integrity": "VALID", "is_valid": true, "anchor_status": "ANCHORED",
  "recalculated_merkle_root": "20e1…", "on_chain_merkle_root": "20e1…" }
```

`integrity` is `VALID`, or `CORRUPTED` if any record changed. If Fabric can't be
reached, the response discloses `anchor_status` so you know the chain wasn't
consulted.

## 2. Trustless local verify (SDK)

Don't trust the server's answer — recompute everything yourself with the
[Go](/aeternis-log/sdks/go/) or [Python](/aeternis-log/sdks/python/) SDK and
compare with the root read from the blockchain:

```python
records = [Record.from_api(r) for r in client.list_records("audit")["records"]]
assert verify_records_locally(records, expected_root="<root from Fabric>")
```

Or fully offline with a CSV export and the CLI (`exit 0` = VALID, `exit 2` =
CORRUPTED) — see [local verification](/aeternis-log/sdks/verification/).

## 3. Public proof (no auth)

For an external auditor who only needs to confirm a batch is anchored:

```bash
curl -s "$BASE/public/anchors/<batch_id>?root=<root>"
```

```json
{ "anchored": true, "anchored_at": "…", "merkle_root": "20e1…", "root_matches": true }
```

The public endpoint exposes only "anchored: yes/no + timestamp + root" — never
tenant metadata. Passing `?root=` checks whether a root you hold matches the
anchored one.

:::note
The public endpoint is **default-identity** scoped. In a deployment that uses
per-tenant signing identities, public verification of those tenants' batches
requires a tenant-scoped token — see [tenant isolation](/aeternis-log/security/tenant-isolation/).
:::
