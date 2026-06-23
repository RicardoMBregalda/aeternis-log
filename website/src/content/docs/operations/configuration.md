---
title: Configuration
description: How AeternisLog is configured — the config file, environment overrides, and the main sections.
sidebar:
  order: 4
---

The API reads `api/config.yaml` and accepts environment-variable overrides (see
`api/.env.example`). Sections: `server`, `mongodb`, `redis`, `fabric`, `wal`,
`batching`, `logging`, `metrics`, `auth`, `rate_limit`, `webhook`.

## Common settings

```yaml
server:
  host: "0.0.0.0"
  port: 5001
  max_body_bytes: 1048576          # request body cap (0 disables)
  cors_allowed_origins: ["*"]      # list explicit origins for credentialed CORS

mongodb:
  url: "mongodb://localhost:27017"
  database: "logdb"

redis:
  host: "localhost"
  cache_enabled: true              # optional cache; degrades gracefully if down

wal:
  enabled: true
  directory: "/var/log/aeternislog-wal"

batching:
  enabled: true
  auto_batch_size: 100
  auto_batch_interval: 30s
```

## Fabric

The API talks to the peer via the **Fabric Gateway gRPC SDK** with an X.509
identity (no Docker socket). Per-tenant channels and identities are optional:

```yaml
fabric:
  channel: "logchannel"
  chaincode: "logchaincode"
  msp_id: "Org1MSP"
  gateway_peer_endpoint: "peer0.org1.example.com:7051"
  identity_cert_file: "/fabric-identity/signcerts/cert.pem"
  identity_key_dir:   "/fabric-identity/keystore"
  # tenant_channels:   { acme: "acme-channel" }
  # tenant_identities: { acme: { identity_cert_file: …, identity_key_dir: … } }
```

## Auth & limits

See [authentication & limits](/aeternis-log/api/authentication/) for `auth` and
`rate_limit`. API keys may be stored hashed (`sha256:<hex>`).

## Environment overrides

Most keys have an env override (e.g. `MONGO_URL`, `REDIS_HOST`, `AUTH_ENABLED`,
`AUTH_API_KEYS`, `RATE_LIMIT_ENABLED`, `WEBHOOK_URL`, `WAL_DIRECTORY`). Env wins
over the file, so the same image is configured per environment without rebuilding.
