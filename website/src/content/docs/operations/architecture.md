---
title: Architecture & components
description: The components you operate when self-hosting AeternisLog and how they fit together.
sidebar:
  order: 1
---

## Components

| Component | What it is | Notes |
|---|---|---|
| **API** | Go service (Gin) | The product. Stateless except for its on-disk WAL. |
| **MongoDB** | Off-chain record store | The queryable hot tier. |
| **Redis** | Cache + shared rate limiter | Optional; the API degrades gracefully without it. |
| **Hyperledger Fabric** | Permissioned ledger | Stores Merkle roots; Go chaincode. External to the API. |
| **WAL** | On-disk write-ahead log | Per-pod; must be on durable storage. |
| **Dashboard** | Static web UI (nginx) | Optional; talks directly to the API. |

## How they connect

```
            ┌─────────────┐      gRPC (gateway)
  client ──▶│   Go API    │──────────────────────▶  Hyperledger Fabric
            └─────┬───┬───┘                          (Merkle roots, write-once)
                  │   └────────▶ Redis (cache + limiter)
                  ▼
              MongoDB (records)        WAL (durable record writes)
```

The API talks to Fabric **only** through the gateway gRPC SDK with an X.509
identity — no Docker socket, no peer CLI.

## Statefulness & scaling

- **API:** mostly stateless, but the file **WAL is per-pod**. A
  ReadWriteOnce PVC cannot be shared across nodes, so multi-node HA needs a
  StatefulSet (per-pod WAL) — the Helm chart gates `replicaCount > 1` behind an
  explicit opt-in.
- **Rate limiting & batching** are already multi-instance safe (Redis-backed
  limiter, atomic batch claim).
- **MongoDB/Redis/Fabric** scale on their own terms (replica sets, clusters,
  multi-org networks).

## Deployment paths

- [Docker Compose](/aeternis-log/operations/docker-compose/) for a single host.
- [Kubernetes (Helm)](/aeternis-log/operations/kubernetes/) for production.
