---
title: Errors
description: The AeternisLog error model — status codes and the error response shape.
sidebar:
  order: 4
---

Errors return a consistent JSON shape with an HTTP status code:

```json
{ "error": "not_found", "message": "no anchor found for the given batch id", "code": 404 }
```

## Status codes

| Code | Meaning |
|---|---|
| `200` | Success. |
| `201` | Record created. |
| `400` | Malformed request (bad JSON, missing fields, invalid domain). |
| `401` | Missing or invalid API key (when auth is enabled). |
| `403` | Authenticated, but not permitted for this tenant/resource. |
| `404` | Record or batch not found. |
| `409` | Conflict — a duplicate record id, or a **`CORRUPTED`** batch on verify. |
| `413` | Request body exceeds `max_body_bytes`. |
| `429` | Rate limit exceeded. |
| `502` | Downstream failure (e.g. the blockchain is unreachable). |
| `500` | Unexpected server error. |

## Notes

- **Integrity failures** surface as `409 CORRUPTED` on the verify endpoint — the
  request succeeded, but the data no longer matches the anchor.
- **Anchor lookups that can't reach Fabric** return `502`, distinct from a `404`
  "not anchored", so verification can disclose when the chain was not consulted.
- The body cap (`413`) and rate limit (`429`) are configurable; see
  [configuration](/aeternis-log/operations/configuration/).
