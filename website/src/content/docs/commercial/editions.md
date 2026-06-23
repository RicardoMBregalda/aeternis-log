---
title: Editions
description: Community vs Enterprise — what's included in each AeternisLog edition.
sidebar:
  order: 1
---

AeternisLog follows an **open-core** model: the Community edition is the full,
self-hostable platform; the Enterprise edition adds the capabilities and
assurances larger deployments need.

## Comparison

| Capability | Community | Enterprise |
|---|:--:|:--:|
| REST API, Merkle batching, on-chain anchoring | ✅ | ✅ |
| Go & Python SDKs + offline verifier CLI | ✅ | ✅ |
| Web integrity dashboard | ✅ | ✅ |
| Hardened v2 hashing (CVE-2012-2459 safe) | ✅ | ✅ |
| Durable WAL, reconciler, write-once anchors | ✅ | ✅ |
| API-key auth, Redis-backed rate limiting | ✅ | ✅ |
| API-key → tenant isolation | ✅ | ✅ |
| Docker Compose + Helm chart | ✅ | ✅ |
| **Ledger-enforced per-tenant identities** | Self-serve | ✅ Guided |
| **Per-tenant Fabric channels** (ledger-per-tenant) | Self-serve | ✅ Guided |
| **Production datastore reference architectures** | Docs | ✅ Reviewed |
| **Multi-org production Fabric network** setup | Scripts | ✅ Assisted |
| **SSO / advanced auth integrations** | — | ✅ |
| **Priority support & SLAs** | Community | ✅ |
| **Security review & deployment assistance** | — | ✅ |
| **Roadmap influence** | — | ✅ |

:::note
This comparison is a starting template. The exact Enterprise feature set and
boundaries are finalized with each customer.
:::

## Which is for you?

- **Community** — evaluate, build internal systems, or run a single-tenant
  deployment you operate yourself.
- **Enterprise** — multi-tenant SaaS, regulated workloads, or any deployment that
  needs ledger-level isolation, production datastores, and support.

See [pricing & contact](/aeternis-log/commercial/pricing/) to discuss a commercial
deployment.
