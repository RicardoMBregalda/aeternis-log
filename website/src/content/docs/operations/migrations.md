---
title: Migrations & upgrades
description: Database schema migrations are a deploy step, not a boot side effect — the API asserts the version and never mutates.
sidebar:
  order: 5
---

Schema changes (indexes today, more later) are **a deploy step, not an API boot
side effect**. The API only *asserts* the expected schema version at boot and
refuses to start on a mismatch — so an upgrade that adds an index can never
silently half-migrate or brick a running deployment.

## The pieces

- `internal/migrations` — an ordered, versioned registry plus a runner. A
  `schema_migrations` collection (one record per applied version, **unique** on
  version) is the source of truth.
- `cmd/migrate` — a standalone binary that applies pending migrations and exits.
  Running it is **idempotent**.
- The API at boot calls `AssertVersion` — read-only.

## How it runs

| Environment | Who migrates |
|---|---|
| docker-compose (dev) | the container entrypoint, when `RUN_MIGRATIONS=true` |
| Kubernetes (prod) | a Helm `pre-install,pre-upgrade` **Job** (`migrate-job.yaml`); the API only asserts |

## Adding a migration

1. Add `internal/migrations/000N_<name>.go` with the next version and an
   idempotent `Up`.
2. Append it to the registry. Never edit, reorder, or remove a released migration.
3. Ship the image — the Job runs first, then the API asserts the new target.

## Rollback

A migration that only **adds** an index/collection is forward-compatible: roll
back by redeploying the previous image and leaving the extra index in place (or
drop it manually if it's a unique constraint the old code violates). Plan
destructive changes as two releases (add-and-backfill, then remove).

## Verifying

```bash
docker exec aeternislog-api /app/aeternislog-migrate   # idempotent; prints from/to/target
# the API logs "database schema verified" with the version on boot
```

Tests drive a real MongoDB: apply reaches target, a re-run is a no-op,
`AssertVersion` fails clearly off-target, and the unique `(tenant,domain,id)` index
is enforced.
