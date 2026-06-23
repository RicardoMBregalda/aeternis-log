---
title: Authentication & limits
description: API-key authentication, rate limiting, and how API keys map to tenants in AeternisLog.
sidebar:
  order: 2
---

API-key authentication and rate limiting are **opt-in** (off by default), so the
sandbox is friction-free. Enable them in production.

## Authentication

```bash
AUTH_ENABLED=true
AUTH_API_KEYS=key-a,key-b        # comma-separated; configure at least one
```

Send the key in `X-API-Key` or as a bearer token:

```bash
curl -H 'X-API-Key: key-a' http://localhost:5001/api/v1/audit/records
curl -H 'Authorization: Bearer key-a' http://localhost:5001/api/v1/audit/records
```

When auth is on, the record and report routes require a key. `/health`, `/`,
`/swagger`, and `/public/anchors/...` stay open.

Keys can be stored **hashed** at rest (`sha256:<hex>`), so the raw key need never
live in config:

```bash
printf '%s' "$KEY" | sha256sum   # -> use "sha256:<hex>" in config
```

## Multi-tenancy

Map API keys to tenants so records are isolated per `(tenant, domain)`:

```yaml
auth:
  enabled: true
  api_keys: ["default-key"]        # flat keys belong to tenant "default"
  tenants:
    - id: "acme"
      keys: ["acme-key-1"]         # or ["sha256:<hex>"]
```

The caller's tenant is resolved from its key; a key for `acme` cannot read tenant
`default`'s records. See [multi-tenancy](/aeternis-log/guides/multi-tenancy/).

## Rate limiting

```bash
RATE_LIMIT_ENABLED=true
RATE_LIMIT_MAX_REQUESTS=100      # per client IP, per window
RATE_LIMIT_WINDOW=1m
```

When Redis is configured the limiter is **shared across instances** (atomic
`INCR`/`PEXPIRE`); without Redis it falls back to per-instance in-memory limiting.
