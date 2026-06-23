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

## Known limitations (must be resolved before per-tenant identities go live)

These do not affect the current single-identity deployment (every batch is under
the default identity), but they bite the moment per-tenant identities or a
chaincode upgrade over existing data are activated:

- **Public verification is default-identity only.** `GET /public/anchors/{id}`
  is unauthenticated and has no tenant, so it queries under the API's default
  identity. A batch anchored under a *per-tenant* identity lives in a different
  composite-key partition and the public endpoint will report it **not anchored**
  even though it is. Until the public path resolves the tenant (the signed,
  tenant-scoped batch token from F16, or a verified mapping from the batch id),
  public proofs only cover default-identity batches. Authenticated verification
  (`POST .../records/verify/{batchId}`) is unaffected — it runs under the tenant.

- **The composite-key change is not backward-compatible.** Batches anchored
  under the pre-F14 `batch_<batchID>` scheme are not found by the tenant-scoped
  `QueryMerkleBatch`/`GetAllMerkleBatches`, so after the chaincode upgrade they
  read as *"does not exist"* and verify reports them unanchored. Deploy the
  upgrade on a **clean ledger**, or plan a one-time migration that re-keys
  existing batches under `(tenant, batchID)` — a naive dual-read fallback to the
  old key is **not** acceptable, since those keys carry no tenant scope and would
  reintroduce the cross-tenant leak this finding closes.
