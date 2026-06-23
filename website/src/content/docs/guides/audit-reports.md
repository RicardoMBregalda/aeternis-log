---
title: Audit reports
description: Generate a per-domain audit report of anchored batches as JSON or a signed PDF.
sidebar:
  order: 5
---

An audit report summarizes a domain's anchored batches — useful for compliance
hand-offs and for the dashboard's overview.

## JSON

```bash
curl -s $BASE/api/v1/audit/report
```

```json
{ "tenant": "default", "domain": "audit", "generated_at": "2026-06-22T22:51:06Z",
  "batches": [
    { "batch_id": "default-audit-3290b38d", "merkle_root": "20e1a987…",
      "tx_id": "1d96e697…", "num_records": 3, "batched_at": "2026-06-22T22:51:05Z" }
  ],
  "total_batches": 1, "total_records": 3 }
```

Each batch carries its Merkle root and the Fabric `tx_id`, so a reviewer can
independently look up the anchor on-chain.

## PDF

```bash
curl -s -o report.pdf $BASE/api/v1/audit/report.pdf
```

A formatted PDF of the same data, suitable for attaching to an audit package or
sending to a regulator.

## Filtering

Reports accept an optional time range so you can scope to a reporting period; the
totals reflect the filtered set.

:::tip
Pair the report with the [public proof](/aeternis-log/guides/verify-integrity/):
the report lists the batches and their roots, and `/public/anchors/{batchId}`
lets anyone confirm each root is anchored — without an API key.
:::
