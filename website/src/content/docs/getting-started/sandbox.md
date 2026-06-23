---
title: Sandbox & smoke test
description: Exercise the full flow with the SDKs and the offline CLI, and run the one-command end-to-end smoke test.
sidebar:
  order: 3
---

The sandbox is the single-org dev network brought up by `make up`. Beyond curl
(see the [Quickstart](/aeternis-log/getting-started/quickstart/)), you can drive
it with the SDKs and verify offline.

## With the Go SDK

```go
import "github.com/RicardoMBregalda/aeternis-log/sdk/go"

c := aeternislog.New("http://localhost:5001")
rec, _ := c.CreateRecord(ctx, "audit", "payments", map[string]any{"event": "charge"}, nil)
batch, _ := c.BatchRecords(ctx, "audit")
res, _ := c.VerifyBatch(ctx, "audit", batch.BatchID) // res.IsValid
```

## With the Python SDK

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

## Offline audit (CLI)

An auditor who holds a CSV export of the records can confirm it matches what was
anchored — fully offline:

```bash
aeternislog verify --file records.csv --api http://localhost:5001 \
  --domain audit --batch-id <batch_id>
# exit 0 = VALID, exit 2 = CORRUPTED
```

CSV columns: `id,timestamp,source,payload`. Row order must match the anchored
batch. See [local verification](/aeternis-log/sdks/verification/).

## One-command smoke test

```bash
make smoke   # full black-box end-to-end suite against the dev API
```

It creates records, anchors them, verifies on-chain, tampers and re-verifies
(expecting `CORRUPTED`), checks the public endpoint, oversized-body rejection, and
reconciliation — a fast confidence check that the whole stack is healthy.

:::note
This sandbox is the single-org dev network. For the multi-organization
production-staging network (3 orgs, Raft ordering, per-tenant channels), see
[Self-hosting → Kubernetes](/aeternis-log/operations/kubernetes/) and the
`hybrid-architecture/fabric-network/prod/` scripts.
:::
