---
title: Backup & disaster recovery
description: What to back up, why the anchor makes recovery verifiable, and how to restore.
sidebar:
  order: 7
---

AeternisLog's anchor turns disaster recovery into a **verifiable** process: after
restoring, you can prove the recovered records are exactly what was anchored.

## What to back up

| Asset | Why | How |
|---|---|---|
| **MongoDB** | The records themselves | Managed snapshots / `mongodump`; a replica set in production. |
| **WAL volume** | Acknowledged-but-not-yet-inserted records | Durable storage (PVC); included in node/volume backups. |
| **Fabric ledger** | The anchors | The network's own ledger backups across orgs/peers. |
| **Identities & keys** | Fabric signing material, API keys | Your secrets manager — back up and rotate. |

## Why recovery is verifiable

Because Merkle roots are anchored independently, you don't have to *trust* a
restored backup — you can **verify** it:

1. Restore MongoDB.
2. For each batch, recompute the Merkle root from the restored records.
3. Compare with the root still on the Fabric ledger.

A match proves the restore is complete and unaltered; a mismatch pinpoints exactly
which batch diverged.

## Restore checklist

1. Restore MongoDB and the WAL volume.
2. Start the API — WAL recovery idempotently replays any un-inserted records.
3. Run schema [migrations](/aeternis-log/operations/migrations/) if the schema
   version advanced.
4. Verify a sample of batches against the on-chain roots (server-side verify, or
   the offline CLI).

## RPO/RTO levers

- **RPO** → the WAL fsync (acknowledged records are durable) + MongoDB snapshot
  cadence.
- **RTO** → managed datastores with fast restore, and a single-replica API that
  comes up in seconds once its WAL volume is attached.
