---
title: Hardening summary
description: The production-hardening outcomes across cryptography, durability, isolation, and the API surface.
sidebar:
  order: 5
---

AeternisLog went through a structured production-hardening pass across the whole
stack. The outcomes, by area:

## Core trust & cryptography

- **Hardened hashing (v2):** length-prefixed fields + leaf/node domain separation,
  versioned per record so old batches still verify.
- **CVE-2012-2459 fixed:** odd Merkle nodes are promoted, not duplicated.
- **Write-once anchors:** the chaincode rejects any overwrite of an anchored root.
- **Conformance vector:** the server and both SDKs are proven to produce
  byte-identical hashes and roots.

## Durability

- **Record-aware WAL:** records are `fsync`-ed before acknowledgement; recovery is
  idempotent — zero acknowledged-write loss.
- **Race-free batch claiming:** concurrent batchers/replicas never double-anchor.
- **Reconciler:** anchor failures are retried, never silently dropped.

## Isolation & scale

- **Ledger-enforced tenant isolation:** batch state is keyed by the caller's
  signed identity (not an argument). See [tenant isolation](/aeternis-log/security/tenant-isolation/).
- **Locked-down public endpoint:** server-side channel resolution; no tenant
  metadata leaks.
- **Shared rate limiter:** Redis-backed (atomic `INCR`/`PEXPIRE`) across instances.
- **Honest Helm chart:** WAL persisted on a PVC; replica gating made explicit.

## API surface & operations

- **Smaller attack surface:** the Docker-socket transport was removed; the API
  talks to Fabric only via the gateway gRPC SDK with an X.509 identity.
- **Assert-not-mutate boot:** schema is migrated out-of-band by a versioned,
  idempotent runner; the API asserts the version and refuses to start on a
  mismatch. See [migrations](/aeternis-log/operations/migrations/).
- **Context-aware anchoring:** request deadlines/cancellation thread through the
  Fabric calls.
- **Dead surface removed:** retired endpoints and unused config knobs were pruned.

## Non-root, least-privilege runtime

- The API runs as a non-root container and mounts only its own least-privilege
  Fabric identity bundle (admin cert + key `0400` + peer TLS CA), not the whole
  organization tree.

For the precise guarantees and their boundaries, see the
[trust model](/aeternis-log/introduction/trust-model/) and
[threat model](/aeternis-log/security/threat-model/).
