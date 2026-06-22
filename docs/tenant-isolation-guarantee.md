# Tenant isolation guarantee (F14)

This document states AeternisLog's multi-tenant isolation guarantee, where it is
enforced, and what is proven under test. It replaces the earlier, weaker reality
in which a tenant was only a string prefix on a batch id sharing one channel and
one identity — which enforced nothing at the ledger.

## The guarantee

> At the ledger boundary, a tenant can neither read, overwrite, nor enumerate
> another tenant's anchored Merkle batches. The tenant is derived from the
> **signed client identity** of the transacting party, never from a function
> argument, so it cannot be forged by a caller.

## Where it is enforced

The guarantee is enforced **in the chaincode**, the one place a client cannot
bypass:

- `clientTenant()` derives the tenant from the client identity — the CA-signed
  `tenant` certificate attribute when present (multi-tenant within one org),
  otherwise the MSP ID (org-per-tenant). It never reads a tenant argument.
- Merkle-batch state is stored under a **composite key `(tenant, batchID)`**.
  `StoreMerkleRoot` and `QueryMerkleBatch` resolve that key from the caller's
  identity; `GetAllMerkleBatches` ranges only over the caller's partition.

Consequences, by construction:
- A batch id that *looks* like another tenant's is still keyed under the caller's
  own identity — the argument cannot widen scope.
- A cross-tenant read resolves to a key that does not exist → reported as
  non-existent (no metadata leak).
- A cross-tenant "overwrite" writes to a different partition → the original
  anchor is untouched; write-once still holds within a tenant.

## Proven under test

`hybrid-architecture/chaincode/logchaincode_isolation_test.go` exercises the
chaincode against a fake stub + client identity and asserts the guarantee:

- store/query are scoped to the tenant; a foreign tenant's read fails;
- a foreign tenant storing the same batch id cannot overwrite the original;
- `GetAllMerkleBatches` returns only the caller's batches;
- the tenant is not spoofable via a tenant-looking batch id;
- isolation also holds by MSP ID when no `tenant` attribute is present.

## Operational requirement (making it real for live tenants)

The enforcement above is unconditional, but for it to isolate *real* tenants in
production each tenant must transact with **its own identity** carrying its
`tenant` attribute. The API supports this directly — it keeps one gateway
session per configured identity and selects by tenant — so the remaining work is
credential provisioning + configuration:

1. Provision a per-tenant identity:
   `prod/scripts/register-tenant-identity.sh <tenant>` registers and enrolls an
   identity whose ecert carries `tenant=<tenant>`.
2. Configure it under `fabric.tenant_identities.<tenant>` (cert + key dir) and
   mount the bundle; the API then anchors that tenant's batches with it (combine
   with the per-tenant channels from F-H where used).
3. Deploy the tenant-scoped chaincode:
   `prod/scripts/upgrade-chaincode.sh <new-version>`.

Until a tenant is configured with its own identity, its anchors use the default
identity and fall back to the MSP-ID partition (org-level isolation), which is
still enforced — never the old shared, unscoped keyspace.
