---
title: API overview
description: Conventions of the AeternisLog REST API — base URL, the generic records surface, content types, and pagination.
sidebar:
  order: 1
---

The API is a small, generic REST surface. Everything is a **record** under a
client-chosen **domain**; there is no per-type endpoint.

- **Base URL:** `http://<host>:5001`
- **Content type:** `application/json`
- **Interactive reference:** Swagger UI at `/swagger/index.html`

## The records surface

| Method | Route | Description |
|---|---|---|
| `POST` | `/api/v1/{domain}/records` | Create a record (hash computed automatically). |
| `GET` | `/api/v1/{domain}/records` | List with `source` filter + pagination. |
| `GET` | `/api/v1/{domain}/records/{id}` | Fetch one record. |
| `DELETE` | `/api/v1/{domain}/records/{id}` | Soft-delete (audit trail preserved). |
| `POST` | `/api/v1/{domain}/records/batch` | Batch pending records, compute the Merkle root, anchor it. |
| `POST` | `/api/v1/{domain}/records/verify/{batchId}` | Verify a batch against the anchored root. |
| `GET` | `/api/v1/{domain}/report` | Audit report (batches + totals), JSON. |
| `GET` | `/api/v1/{domain}/report.pdf` | Audit report, PDF. |

## Operational & public

| Method | Route | Description |
|---|---|---|
| `GET` | `/health` | Service health (Mongo, Redis, Fabric, batch processor). |
| `GET` | `/public/anchors/{batchId}` | Unauthenticated anchor proof for external auditors. |

## Pagination

List endpoints accept `limit`, plus either `offset` or a `cursor` for keyset
pagination, and an optional `source` filter:

```
GET /api/v1/audit/records?limit=50&source=payments
```

## Conventions

- Timestamps are RFC 3339 (UTC).
- Hashes and Merkle roots are lowercase hex SHA-256.
- A successful create returns `{ "status": "success", "data": { … } }`; see
  [errors](/aeternis-log/api/errors/) for the failure shape.

Continue to [Authentication & limits](/aeternis-log/api/authentication/) and the
full [Endpoints](/aeternis-log/api/endpoints/) reference.
