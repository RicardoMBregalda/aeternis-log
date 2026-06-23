---
title: 'Use case: Compliance & audit'
description: Give audit trails and compliance records an independent, tamper-evident proof that holds up to a regulator.
sidebar:
  order: 6
  label: 'Use case: Compliance'
---

## The problem

Regulators increasingly ask not just *"do you keep audit logs?"* but *"can you
prove they weren't altered?"* When logs are rows in a database the operator
controls, the honest answer is "you have to trust us." That is a weak position in
an audit, a dispute, or after a breach.

## How AeternisLog helps

AeternisLog gives every audit record an **independent** integrity proof:

1. **Capture.** Your systems POST compliance events (access, approvals,
   transactions) to a domain such as `audit` — the existing payloads, unchanged.
2. **Anchor.** Records are batched and their Merkle root is anchored write-once on
   a permissioned blockchain the auditor can inspect.
3. **Prove.** At audit time, recompute the root from the stored records and show
   it matches the on-chain anchor. If even one record was altered, the proof
   fails — visibly.

## Why it satisfies auditors

- **Off-vendor anchor.** The proof lives on an independent ledger, not in the same
  database that holds the data.
- **Point-in-time.** Each anchor is timestamped, so it also evidences *when* the
  records existed in that form.
- **Independent verification.** An auditor can verify with the
  [Python CLI](/aeternis-log/sdks/python/) from a CSV export — fully offline,
  trusting only the on-chain root.
- **Tenant isolation.** In a multi-tenant platform, one customer's audit trail is
  cryptographically isolated from another's.

## A concrete flow

```bash
# 1. Your app records a compliance event
POST /api/v1/audit/records   {"source":"access","payload":{"user":"u-91","action":"export","resource":"ledger"}}

# 2. Anchor (or let auto-batching do it)
POST /api/v1/audit/records/batch

# 3. Auditor receives a CSV + the on-chain root, and verifies offline
aeternislog verify --file audit-export.csv --expected-root <root>   # exit 0 = VALID
```

Pair this with [audit reports](/aeternis-log/guides/audit-reports/) (JSON/PDF) for
a complete, hand-off-ready package.
