# Guide — Supply Chain Traceability

In a supply chain, goods pass through many custody steps — manufacturing, packing,
shipping, customs, delivery. Each step produces an event. The trust problem is the
same as for audit logs: how do you prove the **sequence of custody events was not
rewritten retroactively** by a party who controls the database?

This platform anchors each custody event as a hashed record and commits the Merkle root
of each batch to a permissioned blockchain. Any later edit, reorder, or deletion of an
event breaks the recomputed root — and in the multi-org network the anchor is endorsed
by independent organizations, so no single participant can forge the history.

## Model: one domain per shipment (or per product line)

Use the domain to scope a traceable unit — e.g. a shipment id — and record one event per
custody step. The payload carries the step details.

```python
from anchor import Client
c = Client("https://anchor.example.com", api_key="logistics-key")

shipment = "shipment-2026-000123"
for step in [
    {"step": "manufactured", "site": "factory-A", "lot": "L-88", "ts": "2026-06-01T08:00:00Z"},
    {"step": "packed",       "site": "factory-A", "units": 240},
    {"step": "shipped",      "carrier": "ACME-Freight", "container": "C-9001"},
    {"step": "customs",      "port": "Santos", "status": "cleared"},
    {"step": "delivered",    "to": "dc-southeast", "signed_by": "R. Silva"},
]:
    c.create_record(shipment, source="wms", payload=step)

batch = c.batch_records(shipment)   # anchor the custody chain for this shipment
print("anchored:", batch.tx_id, batch.merkle_root)
```

## Verifying a shipment's history

A receiver, regulator, or partner can confirm the recorded custody chain matches what
was anchored — independently of the operator:

```bash
# Export the shipment's events to a CSV (in event order) and verify offline:
anchor verify --file shipment-000123.csv --api https://anchor.example.com \
  --domain shipment-2026-000123 --batch-id <batch_id>
# exit 0 = VALID (history intact), exit 2 = CORRUPTED (an event was altered/reordered)
```

Because order is part of the Merkle root, **reordering events is detected**, not just
edits — which is exactly the property a custody chain needs.

## Why a permissioned, multi-org ledger fits supply chains

- The participants (manufacturer, carrier, distributor) are known organizations — a
  permissioned network (not a public blockchain) matches the trust and privacy model.
- Anchoring requires a **majority of orgs to endorse** (2 of 3 in the staging network),
  so the trace is a shared, distributed guarantee rather than one company's claim.
- Sensitive commercial data stays off-chain (only hashes/roots are anchored); use
  `hash_fields` to anchor just the fields that must be tamper-evident.

## Integration patterns

- **Event-driven**: emit a record from your WMS/ERP on each scan/status change.
- **Batch on cadence**: let the auto-batcher anchor on a timer, or call `batch_records`
  at logical checkpoints (e.g. on `delivered`).
- **Partner verification**: hand partners the CSV export + the on-chain root; they run
  `anchor verify` themselves — no access to your systems required.

See also the [Sandbox Quickstart](sandbox-quickstart.md) and the SDK READMEs
([Go](../../sdk/go/README.md), [Python](../../sdk/python/README.md)).
