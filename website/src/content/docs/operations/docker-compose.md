---
title: Docker Compose
description: Run AeternisLog on a single host with Docker Compose, including the schema-migration step.
sidebar:
  order: 2
---

The Compose stack is the fastest way to run everything on one host.

## Bring it up

```bash
make up        # Fabric network + API + MongoDB + Redis + dashboard
make status
make api-logs  # follow the API logs
```

The API image builds both the API and the **migrator** binary, and the
entrypoint applies migrations before the API starts when `RUN_MIGRATIONS=true`
(set in `api/docker-compose.yml` for development):

```yaml
environment:
  - MONGO_URL=mongodb://mongodb:27017
  - WAL_DIRECTORY=/var/log/aeternislog-wal
  - RUN_MIGRATIONS=true   # apply migrations before the API starts (dev convenience)
```

## Services

- `go-api` — the API (ports `5001`, metrics `9090`).
- `mongodb` — record store (volume `mongodb-data`).
- `redis` — cache + limiter (AOF `appendfsync always` for durability).
- `dashboard` — nginx serving `examples/dashboard` on `8088`.

The WAL lives on the `wal-data` volume, so acknowledged records survive container
restarts.

## Production notes

- In Kubernetes, **do not** run migrations from the API entrypoint — use the
  pre-upgrade [migration Job](/aeternis-log/operations/migrations/) and let the API
  only *assert* the schema version.
- The in-Compose MongoDB and Redis are single pods for evaluation. For production,
  point at managed/replicated datastores — see
  [configuration](/aeternis-log/operations/configuration/).

## Tear down

```bash
make down    # stop, keep data
make clean   # stop and remove volumes
```
