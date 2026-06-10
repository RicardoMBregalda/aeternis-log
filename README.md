# AeternisLog — Tamper-Evident Data Anchoring

**AeternisLog** gives any data **cryptographic proof of integrity**. Records go to MongoDB (fast, queryable), are grouped into **Merkle Trees**, and the root of each batch is anchored on **Hyperledger Fabric** (immutable, auditable). A **Write-Ahead Log (WAL)** with `fsync` guarantees zero data loss before any processing.

Any tampering is mathematically detectable: an auditor recomputes the Merkle Root from MongoDB and compares it against what is stored on the blockchain.

> **Product vertical:** Compliance & Audit Trail. Direction and plan in [ROADMAP.md](ROADMAP.md).

## Architecture

```
POST /logs ──► WAL (fsync) ──► MongoDB ──► batch (Merkle root) ──► anchor on Fabric
```

| Component | Role |
|---|---|
| **Go API** (`api/`) | REST (Gin), structured logging (zerolog), WAL, Merkle Tree batching |
| **MongoDB** | Off-chain storage for logs (hot tier) |
| **Redis** | Optional cache (graceful degradation if absent) |
| **Hyperledger Fabric** (`hybrid-architecture/`) | Permissioned blockchain (Raft consensus) that stores the Merkle roots; chaincode in Go |
| **WAL** | Durability (0% loss). Two backends: `file` (`O_SYNC` + `fsync`, single instance) or `redis` (Redis Streams consumer group, multiple API instances) |

## Prerequisites

- Docker + Docker Compose
- Go 1.18+ (for native build/test)
- `make`

## Quick start

```bash
make up        # brings up EVERYTHING: blockchain (Fabric) + API + MongoDB + Redis
make down      # stops everything
make help      # lists all commands
```

For development without the blockchain:

```bash
make dev       # brings up only MongoDB + Redis
make run       # runs the API natively (Go)
# or
make api       # runs the API in a container (without Fabric)
```

The API comes up at **http://localhost:5001** — Swagger at `/swagger/index.html`, health at `/health`.

> The Fabric network and the API share the Docker network `aeternislog_network`, created automatically by `make`. Its absence was previously what made `docker compose up` create nothing.

## Project structure

```
.
├── api/                      # Go API (the product)
│   ├── cmd/api/              # entrypoint
│   ├── internal/             # handlers, database, fabric, merkle, wal, logger, ...
│   ├── pkg/config/           # configuration
│   ├── Dockerfile
│   └── docker-compose.yml    # API + MongoDB + Redis
├── hybrid-architecture/
│   ├── chaincode/            # smart contract (Go)
│   └── fabric-network/       # Fabric network (peers, orderer, CA, scripts)
├── Makefile                  # orchestration (make up / down / dev / ...)
├── ROADMAP.md
└── README.md
```

## API — main endpoints

| Method | Route | Description |
|---|---|---|
| `POST` | `/logs` | Create a log (automatic hash + WAL) |
| `GET` | `/logs` | List with filters (`source`, `level`, `limit`) and pagination — `offset` or keyset `cursor` (response returns `next_cursor`) |
| `GET` | `/logs/:id` | Fetch by ID |
| `POST` | `/merkle/force-batch` | Force batch creation |
| `POST` | `/merkle/verify/:id` | Verify the integrity of a batch (Merkle proof) |
| `GET` | `/merkle/batches` | List batches |
| `GET` | `/health` · `/stats` | Health and statistics |

Example:

```bash
curl -X POST http://localhost:5001/logs \
  -H 'Content-Type: application/json' \
  -d '{"source": "auth-service", "level": "INFO", "message": "User login successful"}'
```

## Records (generic domain)

Beyond logs, the API anchors **any** auditable record under a client-chosen domain — the core of the *Tamper-Evident Data Anchoring* pattern. A `Log` is just a record in the `logs` domain.

| Method | Route | Description |
|---|---|---|
| `POST` | `/api/v1/{domain}/records` | Create a record (`payload` is any JSON; integrity `hash` computed automatically) |
| `GET` | `/api/v1/{domain}/records` | List with filters (`source`) and pagination (`offset` or `cursor`) |
| `GET` | `/api/v1/{domain}/records/:id` | Fetch by ID |
| `DELETE` | `/api/v1/{domain}/records/:id` | Soft-delete (audit trail preserved) |
| `POST` | `/api/v1/{domain}/records/batch` | Batch pending records, compute the Merkle root and anchor it to Fabric |
| `POST` | `/api/v1/{domain}/records/verify/:batchId` | Verify a batch's integrity (recompute vs the anchored root) |

```bash
curl -X POST http://localhost:5001/api/v1/contracts/records \
  -H 'Content-Type: application/json' \
  -d '{"source": "crm", "payload": {"party": "acme", "amount": 100}, "hash_fields": ["party", "amount"]}'
```

The hash is `SHA-256(id | timestamp | source | canonical(payload))`. Optional `hash_fields` restricts which payload keys feed the hash (so other fields can change without breaking it); the canonical payload uses sorted keys, so the hash is independent of key order.

Records are batched per domain (automatically, or via `POST .../records/batch`), their Merkle root is anchored on Fabric, and `POST .../records/verify/{batchId}` recomputes the root from current content and compares it with the anchored one — returning `409 CORRUPTED` if anything was tampered.

**Webhook:** set `webhook.enabled` + `webhook.url` (or `WEBHOOK_ENABLED`/`WEBHOOK_URL`) to receive a `batch.anchored` callback every time a batch — logs or records — is anchored. The JSON payload (`domain`, `batch_id`, `merkle_root`, `num_records`, `tx_id`, `anchored_at`) is signed with HMAC-SHA256 in `X-Webhook-Signature` when a `webhook.secret` is set; delivery is async with retries.

**Go SDK:** [`sdk/go`](sdk/go) (package `aeternislog`, standard-library only) wraps the API with automatic retries and recomputes record hashes / Merkle roots **locally**, so integrity can be verified without trusting the server. See [sdk/go/README.md](sdk/go/README.md).

## Authentication & rate limiting

API key authentication and rate limiting are **opt-in** (off by default). Enable them in production:

```bash
AUTH_ENABLED=true
AUTH_API_KEYS=key-a,key-b          # comma-separated; configure at least one
RATE_LIMIT_ENABLED=true
RATE_LIMIT_MAX_REQUESTS=100        # per client IP, per window
RATE_LIMIT_WINDOW=1m
```

When auth is enabled, the data routes (`/logs`, `/merkle`, `/wal`, `/stats`) require a key; `/health`, `/` and `/swagger` stay open. Send the key in `X-API-Key` or as a bearer token:

```bash
curl -H 'X-API-Key: key-a' http://localhost:5001/logs
curl -H 'Authorization: Bearer key-a' http://localhost:5001/logs
```

Rate limiting is in-memory **per instance**; a Redis-backed shared limiter is the follow-up for multi-instance deployments.

**Multi-tenancy:** map API keys to tenants via `auth.tenants` (`[{id, keys}]`); flat `api_keys` belong to tenant `default`. The caller's tenant is resolved from its key, and records are isolated per `(tenant, domain)` — a key for tenant A cannot read tenant B's records. (Per-org Fabric channels are a production-network follow-up; today tenants anchor to the same channel, namespaced by tenant in the batch id.)

## Configuration

The API reads `api/config.yaml` and accepts overrides via environment variables (see `api/.env.example`). Sections: `server`, `mongodb`, `redis`, `fabric`, `wal`, `batching`, `logging`, `metrics`, `auth`, `rate_limit`.

**Running multiple API instances:** set `wal.backend: redis` (or `WAL_BACKEND=redis`). The WAL then uses a Redis Streams consumer group, so each log entry is processed by exactly one instance and entries from a crashed instance are reclaimed automatically. In this mode durability depends on Redis persistence — run Redis with AOF enabled (`appendonly yes`; `appendfsync always` for parity with the file backend's per-write `fsync`). The default `file` backend keeps the original single-instance behavior.

**Fabric transport:** the API talks to the peer via `fabric.transport`. `gateway` (default) uses the Fabric Gateway gRPC SDK with an X.509 identity and needs no Docker socket; `docker-exec` is the legacy fallback that shells out to the peer CLI. The gateway paths default to the docker-compose `/fabric-crypto` mount — for a native run (outside compose) set `FABRIC_TRANSPORT=docker-exec`, or override the gateway endpoint/paths (see `api/.env.example`).

## Development

```bash
make build     # compiles the API
make test      # go test ./...
make vet       # go vet ./...
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
