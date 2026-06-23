---
title: Integrity dashboard
description: A dependency-free web dashboard to create records, anchor batches, and verify integrity — including the unauthenticated public proof.
sidebar:
  order: 4
---

AeternisLog ships a dependency-free web **integrity dashboard** that exercises the
whole flow against the API — no build step, just a static page served by nginx.

`make up` serves it at **http://localhost:8088**.

## What it shows

- **Live health** — Mongo / Redis / Fabric / batch-processor status.
- **Overview** — records, anchored batches, anchored records, and services up
  (from the domain audit report).
- **Records** — list and filter by source, create a record inline, and **anchor
  the pending pool** into a Merkle batch.
- **Anchored batches** — each batch's Merkle root and `tx_id`, with one-click
  **verify** (recompute server-side vs the on-chain anchor) and the
  unauthenticated **public proof** an external auditor uses.
- **Public verification** — prove a batch is anchored, and optionally check that a
  Merkle root you hold matches the anchored one — without an API key.

## Run it standalone

```bash
make dashboard                                               # nginx on :8088
# or, without Docker:
python3 -m http.server 8088 --directory examples/dashboard
```

Enter the API base URL (e.g. `http://localhost:5001`), the records **domain**,
and — if auth is enabled — an API key. Connection settings persist in the
browser's `localStorage`.

:::tip
The dashboard talks **directly** to the API at its published port (CORS is
permissive by default). For production, serve it behind the same origin as the
API and scope CORS accordingly.
:::
