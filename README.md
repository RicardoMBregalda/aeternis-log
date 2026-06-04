# Log Management — Tamper-Evident Log Anchoring

Log management API with **cryptographic proof of integrity**. Logs go to MongoDB (fast, queryable), are grouped into **Merkle Trees**, and the root of each batch is anchored on **Hyperledger Fabric** (immutable, auditable). A **Write-Ahead Log (WAL)** with `fsync` guarantees zero data loss before any processing.

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
| **WAL** | Durability (0% loss) with `O_SYNC` + `fsync` |

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

> The Fabric network and the API share the Docker network `tcc_log_network`, created automatically by `make`. Its absence was previously what made `docker compose up` create nothing.

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
| `GET` | `/logs` | List with filters (`source`, `level`, `limit`, `offset`) |
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

## Configuration

The API reads `api/config.yaml` and accepts overrides via environment variables (see `api/.env.example`). Sections: `server`, `mongodb`, `redis`, `fabric`, `wal`, `batching`, `logging`, `metrics`.

## Development

```bash
make build     # compiles the API
make test      # go test ./...
make vet       # go vet ./...
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
