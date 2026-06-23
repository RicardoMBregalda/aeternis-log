---
title: Multi-tenancy
description: How AeternisLog isolates tenants — at the API by key, and at the ledger boundary by signed identity.
sidebar:
  order: 3
---

AeternisLog is multi-tenant at two layers.

## At the API: keys → tenants

Records are isolated per `(tenant, domain)`. A caller's tenant is resolved from
its API key; a key for one tenant cannot read another's records.

```yaml
auth:
  enabled: true
  api_keys: ["default-key"]        # flat keys → tenant "default"
  tenants:
    - id: "acme"
      keys: ["acme-key-1"]
    - id: "globex"
      keys: ["globex-key-1"]
```

This holds as long as all access goes through the API.

## At the ledger: identity-enforced isolation

For isolation that does **not** depend on trusting the API, AeternisLog enforces
it in the chaincode itself. The chaincode derives the tenant from the caller's
**signed client identity** (a CA-signed `tenant` certificate attribute, else the
MSP ID) — never from a function argument — and keys Merkle-batch state by a
composite `(tenant, batchID)`.

By construction:

- a tenant **cannot read** another tenant's anchored batch (it resolves to a key
  that doesn't exist);
- a tenant **cannot overwrite** another's batch (it writes to a different
  partition);
- a tenant **cannot enumerate** another's batches.

The tenant cannot be forged through a batch id that merely *looks* like another
tenant's — only the signed identity governs the scope.

## Per-tenant channels (optional)

Tenants can also anchor to a **dedicated Fabric channel** (a separate ledger) via
`fabric.tenant_channels`, raising isolation from a partition to a full ledger
boundary.

## Making it live

To use a per-tenant identity end to end:

1. Provision a per-tenant identity (`prod/scripts/register-tenant-identity.sh`)
   whose certificate carries `tenant=<id>`.
2. Map it under `fabric.tenant_identities.<id>` and mount the bundle.

Until a tenant has its own identity, its anchors fall back to the MSP-level
partition — still enforced, never the old shared keyspace. See the full
[tenant isolation guarantee](/aeternis-log/security/tenant-isolation/).
