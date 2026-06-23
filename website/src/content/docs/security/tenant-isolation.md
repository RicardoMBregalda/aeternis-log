---
title: Tenant isolation
description: How AeternisLog enforces multi-tenant isolation at the ledger boundary, by signed identity rather than a string prefix.
sidebar:
  order: 3
---

## The guarantee

> At the ledger boundary, a tenant can neither read, overwrite, nor enumerate
> another tenant's anchored Merkle batches. The tenant is derived from the
> **signed client identity** of the transacting party, never from a function
> argument — so it cannot be forged by a caller.

This replaces a weaker earlier reality where a "tenant" was only a string prefix
on a batch id sharing one channel and one identity, which enforced nothing at the
ledger.

## Where it is enforced

In the **chaincode**, the one place a client cannot bypass:

- `clientTenant()` derives the tenant from the client identity — the CA-signed
  `tenant` certificate attribute when present (multi-tenant within one org),
  otherwise the MSP ID (org-per-tenant). It never reads a tenant argument.
- Merkle-batch state is stored under a **composite key `(tenant, batchID)`**.
  Store/query resolve that key from the caller's identity; listing ranges only
  over the caller's partition.

By construction:

- a batch id that *looks* like another tenant's is still keyed under the caller's
  own identity — the argument cannot widen scope;
- a cross-tenant read resolves to a key that does not exist → reported as
  non-existent (no metadata leak);
- a cross-tenant "overwrite" writes to a different partition → the original anchor
  is untouched; write-once still holds within a tenant.

## Proven under test

The chaincode is tested against a fake stub + client identity asserting: scoped
store/query, no foreign read, no cross-tenant overwrite, listing returns only the
caller's batches, the tenant is not spoofable via a batch id, and isolation also
holds by MSP ID without an attribute.

## Operational requirement

For real tenants, each must transact with **its own identity** carrying its
`tenant` attribute — a credential-provisioning step
(`register-tenant-identity.sh`) plus a `fabric.tenant_identities` mapping. Until
then, anchors use the default identity and fall back to the MSP-ID partition
(org-level isolation) — still enforced, never the old shared keyspace.

:::caution[Known limitations]
The unauthenticated public-verify endpoint is default-identity scoped, so it can't
see batches anchored under a per-tenant identity until a tenant-scoped token is
added. And the composite-key change is not backward-compatible with batches
anchored under the pre-isolation key scheme — deploy the chaincode upgrade on a
clean ledger or plan a re-keying migration.
:::
