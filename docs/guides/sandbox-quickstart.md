# Sandbox Quickstart — evaluate in minutes

Spin up the full stack locally — a Hyperledger Fabric network plus the API (Go +
MongoDB + Redis) — and exercise the whole tamper-evident flow: create records, anchor
them on the blockchain, and verify integrity independently.

## Prerequisites

- Docker + Docker Compose
- (optional) Go 1.21+ and Python 3.8+ to use the SDKs natively

## 1. Bring up the stack

```bash
make up        # Fabric network + API + MongoDB + Redis
make status    # list the running containers
```

When it finishes:

- API: http://localhost:5001
- Health: http://localhost:5001/health
- Swagger: http://localhost:5001/swagger/index.html
- Metrics: http://localhost:9090/metrics

## 2. Anchor and verify (curl)

```bash
# Create a couple of records in the "audit" domain.
curl -s -XPOST http://localhost:5001/api/v1/audit/records \
  -H 'Content-Type: application/json' \
  -d '{"source":"payments","payload":{"event":"charge","amount":1200}}'

# Batch the domain's pending records and anchor the Merkle root on the blockchain.
curl -s -XPOST http://localhost:5001/api/v1/audit/records/batch \
  -H 'Content-Type: application/json' -d '{"batch_size":100}'
# -> {"batch_id":"...","merkle_root":"...","tx_id":"...","anchored":true,...}

# Verify the batch (200 VALID; 409 CORRUPTED if any record changed).
curl -s -XPOST http://localhost:5001/api/v1/audit/records/verify/<batch_id>
```

## 3. Use an SDK

Go:

```go
import "github.com/RicardoMBregalda/aeternis-log/sdk/go"

c := aeternislog.New("http://localhost:5001")
rec, _ := c.CreateRecord(ctx, "audit", "payments", map[string]any{"event": "charge"}, nil)
batch, _ := c.BatchRecords(ctx, "audit")
res, _ := c.VerifyBatch(ctx, "audit", batch.BatchID) // res.IsValid
```

Python:

```bash
pip install ./sdk/python
```
```python
from aeternislog import Client
c = Client("http://localhost:5001")
rec = c.create_record("audit", source="payments", payload={"event": "charge"})
batch = c.batch_records("audit")
assert c.verify_batch("audit", batch.batch_id).is_valid
```

## 4. Offline audit (CLI)

```bash
# Export a batch's records to records.csv (id,timestamp,source,payload), then:
aeternislog verify --file records.csv --api http://localhost:5001 --domain audit --batch-id <batch_id>
# exit 0 = VALID, exit 2 = CORRUPTED
```

## 5. Dashboard

```bash
python3 -m http.server 8088 --directory examples/dashboard   # then open http://localhost:8088
```

## 6. One-command smoke test

```bash
make smoke   # runs the full black-box end-to-end suite against the dev API
```

## Tear down

```bash
make down    # stop, keep data
make clean   # stop and remove volumes (Mongo/Redis/WAL + Fabric)
```

> This sandbox is the single-org dev network. For the multi-organization
> production-staging network (3 orgs, Raft, per-tenant channels), see
> [docs/plano-rede-fabric-producao.md](../plano-rede-fabric-producao.md) and
> `hybrid-architecture/fabric-network/prod/`.
