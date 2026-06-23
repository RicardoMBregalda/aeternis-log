# Runbook — schema migrations, chaincode upgrades, datastores

This runbook covers how AeternisLog changes its database schema and chaincode
without bricking a running deployment, and what the shipped datastores are (and
are not) suitable for. It is the operational counterpart to finding F18.

## 1. Database schema migrations

Schema changes (indexes today, collections/fields later) are **a deploy step,
not an API boot side effect**. The pieces:

- `api/internal/migrations` — an ordered, versioned registry of idempotent
  migrations plus a runner. `schema_migrations` (one document per applied
  version) is the single source of truth for the current version.
- `api/cmd/migrate` — a standalone binary that applies every pending migration
  and exits. Running it is **idempotent**: with nothing pending it applies
  nothing and exits 0.
- The **API only asserts** the expected version at boot and refuses to start on
  a mismatch (`migrations.AssertVersion`). It never mutates the schema, so an
  upgrade that adds an index can never silently half-migrate or brick a pod.

### How it runs

| Environment | Who migrates | How |
|---|---|---|
| docker-compose (dev) | the container entrypoint | `RUN_MIGRATIONS=true` runs `/app/aeternislog-migrate` before the API |
| Kubernetes (prod) | a Helm Job | `templates/migrate-job.yaml`, a `pre-install,pre-upgrade` hook (weight -5), runs before the API rolls out; the API Deployment leaves `RUN_MIGRATIONS` unset and only asserts |

### Adding a migration

1. Add a file `api/internal/migrations/000N_<name>.go` with the next version and
   an idempotent `Up`.
2. Append it to the `registry` in `migrations.go`. Never edit, reorder, or
   remove a released migration.
3. Ship the new image. On deploy the migration Job runs first; the API then
   asserts the new `TargetVersion()`.

### Rollback

A migration that only **adds** an index or collection is forward-compatible: an
older API asserts a lower `TargetVersion()` and will refuse to start against the
newer schema, so roll back by redeploying the previous image **and** leaving the
extra index in place (it is harmless to the old version's queries), or drop the
index manually if it is a unique constraint the old code violates. Plan
destructive migrations (dropping/renaming) as two releases: add-and-backfill
first, remove only once no running version needs the old shape.

### Verifying

```bash
# current applied version
docker exec aeternislog-api /app/aeternislog-migrate   # idempotent; prints from/to/target
# the API logs "database schema verified" with schema_version on boot
```

Tests in `api/internal/migrations/runner_test.go` drive a real MongoDB: apply
reaches target, a re-run is a no-op, `AssertVersion` fails clearly off-target,
and the unique `(tenant, domain, id)` index is enforced.

## 2. Chaincode upgrades

Chaincode changes (e.g. the F14 tenant-scoped keys) ship via the Fabric
chaincode lifecycle at a **new version and an incremented sequence**. The world
state is preserved; only the definition/binary changes.

```bash
# inside the CLI container of the 3-org network
hybrid-architecture/fabric-network/prod/scripts/upgrade-chaincode.sh <new-version> [new-sequence]
# e.g. deploying the F14 chaincode:
hybrid-architecture/fabric-network/prod/scripts/upgrade-chaincode.sh 2.0
```

The script derives the next sequence from what is committed, then runs
package → install on every org → approve for all 3 orgs → commit under MAJORITY
(2 of 3) endorsement. It is the upgrade counterpart of the initial
`deploy-chaincode.sh`.

Because anchors are write-once, an upgrade never rewrites existing batches; new
behavior (e.g. tenant-scoped keys) applies to batches anchored after the commit.
Sequence with the schema migration above when a change spans both layers.

> **Breaking change — F14 tenant-scoped keys.** The F14 upgrade moves batch
> state from `batch_<batchID>` to a composite `(tenant, batchID)` key. Batches
> anchored under the old scheme are **not** readable by the new chaincode (verify
> reports them unanchored). Apply this upgrade on a clean ledger, or plan a
> one-time re-keying migration first. See `docs/tenant-isolation-guarantee.md`
> ("Known limitations").

## 3. Datastores

The Helm chart ships **demo single-pod** MongoDB and Redis for evaluation. They
are **not** production-grade:

- **MongoDB** — single pod, single PVC, no replica set, no managed backups. For
  production run an external managed replica set: set `mongodb.enabled=false`
  and point `mongodb.url` at it. The migration Job and the API read the same
  `config.yaml`, so both follow that URL.
- **Redis** — single pod, no HA. Redis is an optional cache/limiter; the API
  degrades gracefully without it. For production use an external managed Redis
  (`redis.enabled=false`).

Do not present the in-chart single pods as production storage.
