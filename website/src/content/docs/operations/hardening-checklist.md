---
title: Production hardening checklist
description: A checklist to take an AeternisLog deployment from sandbox to production.
sidebar:
  order: 9
---

A pragmatic checklist for a production deployment. Many items are already the
chart's defaults — this is what to confirm.

## Identity & secrets

- [ ] **Auth enabled** (`AUTH_ENABLED=true`) with at least one key; keys stored
      **hashed** (`sha256:<hex>`).
- [ ] **Rate limiting enabled**, Redis-backed (shared across instances).
- [ ] Fabric signing identity is **least-privilege** (own bundle, key `0400`),
      not the whole org tree.
- [ ] Secrets in a secrets manager, not in ConfigMaps or images; rotation planned.

## Data & durability

- [ ] **WAL on durable storage** (PVC), not an emptyDir.
- [ ] **External managed MongoDB** (replica set + backups); `mongodb.enabled=false`.
- [ ] Backup & restore tested — and verified against on-chain roots
      (see [backup & DR](/aeternis-log/operations/backup-dr/)).

## Schema & chaincode

- [ ] Migrations run by the **pre-upgrade Job**, not the API entrypoint; the API
      asserts the version.
- [ ] Chaincode upgrade path scripted and rehearsed
      ([chaincode lifecycle](/aeternis-log/operations/chaincode-upgrades/)).

## Isolation

- [ ] Per-tenant **identities** provisioned and mapped (`fabric.tenant_identities`)
      if you run multiple tenants — not just API-key isolation.
- [ ] Per-tenant **channels** considered for the strongest ledger isolation.

## Network & surface

- [ ] API behind TLS / an ingress with your own hostname.
- [ ] **CORS** scoped to known origins (not `*`) if browsers call it with
      credentials.
- [ ] Request **body cap** and timeouts set for your workload.

## Observability

- [ ] `/health` wired to liveness/readiness; Prometheus scraping `:9090/metrics`.
- [ ] Alerts on anchor failures, batch backlog, error rate, Fabric health.
- [ ] `make smoke` (or equivalent) on a schedule.

## Scale

- [ ] Single API replica **or** per-pod WAL persistence arranged before setting
      `allowMultiReplica=true`.
