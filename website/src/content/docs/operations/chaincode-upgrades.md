---
title: Chaincode lifecycle
description: Upgrade the AeternisLog chaincode via the Fabric lifecycle at an incremented sequence, preserving world state.
sidebar:
  order: 6
---

Chaincode changes ship via the Fabric chaincode lifecycle at a **new version and
an incremented sequence**. World state is preserved; only the definition/binary
changes.

## Upgrade

```bash
# inside the CLI container of the network
hybrid-architecture/fabric-network/prod/scripts/upgrade-chaincode.sh <new-version> [new-sequence]

# e.g.
upgrade-chaincode.sh 2.0
```

The script derives the next sequence from what is committed, then runs **package →
install on every org → approve (all orgs) → commit** under the channel's
endorsement policy (MAJORITY, 2 of 3 on the staging network). A committed chaincode
whose sequence can't be parsed fails with a clear error rather than silently
restarting at sequence 1.

## Anchors are preserved

Because anchors are write-once, an upgrade never rewrites existing batches; new
behavior applies to batches anchored after the commit. Sequence chaincode upgrades
with [schema migrations](/aeternis-log/operations/migrations/) when a change spans
both layers.

:::caution[Tenant-isolation upgrade is breaking]
The ledger-isolation chaincode moves batch state from a flat `batch_<batchID>` key
to a composite `(tenant, batchID)` key. Batches anchored under the old scheme are
**not** readable by the new chaincode (verify reports them unanchored). Apply this
upgrade on a clean ledger, or plan a one-time re-keying migration first — a naive
dual-read would reintroduce cross-tenant leakage of the un-scoped old batches.
:::

## Per-tenant identities

Provision a per-tenant signing identity (whose ecert carries `tenant=<id>`) with
`register-tenant-identity.sh`, then map it under `fabric.tenant_identities`. See
[tenant isolation](/aeternis-log/security/tenant-isolation/).
