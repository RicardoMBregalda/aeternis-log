# Log Management — Tamper-Evident Log Anchoring

API de gerenciamento de logs com **prova criptográfica de integridade**. Os logs vão para o MongoDB (rápido, consultável), são agrupados em **Merkle Trees**, e a raiz de cada lote é ancorada no **Hyperledger Fabric** (imutável, auditável). Um **Write-Ahead Log (WAL)** com `fsync` garante zero perda de dados antes de qualquer processamento.

Qualquer adulteração é detectável matematicamente: um auditor recalcula o Merkle Root a partir do MongoDB e compara com o que está na blockchain.

> **Vertical de produto:** Compliance & Audit Trail. Direção e plano em [ROADMAP.md](ROADMAP.md).

## Arquitetura

```
POST /logs ──► WAL (fsync) ──► MongoDB ──► batch (Merkle root) ──► âncora no Fabric
```

| Componente | Papel |
|---|---|
| **API Go** (`api/`) | REST (Gin), logging estruturado (zerolog), WAL, batching por Merkle Tree |
| **MongoDB** | Armazenamento off-chain dos logs (camada quente) |
| **Redis** | Cache opcional (degradação graciosa se ausente) |
| **Hyperledger Fabric** (`hybrid-architecture/`) | Blockchain permissionada (consenso Raft) que guarda as raízes de Merkle; chaincode em Go |
| **WAL** | Durabilidade (0% de perda) com `O_SYNC` + `fsync` |

## Pré-requisitos

- Docker + Docker Compose
- Go 1.18+ (para build/test nativo)
- `make`

## Início rápido

```bash
make up        # sobe TUDO: blockchain (Fabric) + API + MongoDB + Redis
make down      # para tudo
make help      # lista todos os comandos
```

Para desenvolvimento sem a blockchain:

```bash
make dev       # sobe só MongoDB + Redis
make run       # roda a API nativamente (Go)
# ou
make api       # sobe a API em container (sem o Fabric)
```

A API sobe em **http://localhost:5001** — Swagger em `/swagger/index.html`, health em `/health`.

> A rede Fabric e a API compartilham a rede Docker `tcc_log_network`, criada automaticamente pelo `make`. Era a ausência dela que antes fazia o `docker compose up` não criar nada.

## Estrutura do projeto

```
.
├── api/                      # API Go (o produto)
│   ├── cmd/api/              # entrypoint
│   ├── internal/             # handlers, database, fabric, merkle, wal, logger, ...
│   ├── pkg/config/           # configuração
│   ├── Dockerfile
│   └── docker-compose.yml    # API + MongoDB + Redis
├── hybrid-architecture/
│   ├── chaincode/            # smart contract (Go)
│   └── fabric-network/       # rede Fabric (peers, orderer, CA, scripts)
├── Makefile                  # orquestração (make up / down / dev / ...)
├── ROADMAP.md
└── README.md
```

## API — principais endpoints

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/logs` | Cria um log (hash automático + WAL) |
| `GET` | `/logs` | Lista com filtros (`source`, `level`, `limit`, `offset`) |
| `GET` | `/logs/:id` | Busca por ID |
| `POST` | `/merkle/force-batch` | Força a criação de um lote |
| `POST` | `/merkle/verify/:id` | Verifica a integridade de um lote (Merkle proof) |
| `GET` | `/merkle/batches` | Lista lotes |
| `GET` | `/health` · `/stats` | Saúde e estatísticas |

Exemplo:

```bash
curl -X POST http://localhost:5001/logs \
  -H 'Content-Type: application/json' \
  -d '{"source": "auth-service", "level": "INFO", "message": "User login successful"}'
```

## Configuração

A API lê `api/config.yaml` e aceita override por variáveis de ambiente (ver `api/.env.example`). Seções: `server`, `mongodb`, `redis`, `fabric`, `wal`, `batching`, `logging`, `metrics`.

## Desenvolvimento

```bash
make build     # compila a API
make test      # go test ./...
make vet       # go vet ./...
```

## Licença

Apache 2.0 — ver [LICENSE](LICENSE).
