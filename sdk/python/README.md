# anchor-sdk (Python)

Python client and **offline verifier** for the Tamper-Evident Data Anchoring API.

Like the [Go SDK](../go), it does more than wrap the REST API: it recomputes record
hashes and Merkle roots **locally and independently of the server**, so an auditor can
verify integrity without trusting the API. Standard library only — no dependencies.

## Install

```bash
pip install anchor-sdk          # once published
# or, from this repo:
pip install ./sdk/python
```

## Library

```python
from anchor import Client

client = Client("http://localhost:5001", api_key="my-key")  # api_key optional

# Create a record. The id/timestamp are generated client-side and the server's
# returned hash is checked against the independently computed one (raises on mismatch).
rec = client.create_record("audit", source="payments", payload={"event": "charge", "amount": 1200})

# Batch the domain's pending records and anchor the Merkle root on the blockchain.
batch = client.batch_records("audit")
print(batch.tx_id, batch.merkle_root, batch.channel)

# Verify server-side (a CORRUPTED batch returns is_valid=False, it does not raise).
result = client.verify_batch("audit", batch.batch_id)
assert result.is_valid
```

### Trustless local verification

```python
from anchor import Record, merkle_root, verify_records_locally

records = [Record.from_api(r) for r in client.list_records("audit")["records"]]

# Recompute each hash and the root yourself; compare with the anchored root.
assert verify_records_locally(records, expected_root="<root from the blockchain>")
```

`Record.compute_hash()` reproduces the server algorithm byte-for-byte:

```
SHA-256(id + timestamp + source + canonical(payload))
```

`canonical(payload)` matches Go's `encoding/json` (sorted keys, compact, HTML-escaped),
so hashes are identical across the Go server, the Go SDK and this SDK.

> Payload numbers are decoded as 64-bit floats server-side, so integer-valued numbers are
> canonicalized without a decimal point (`7`, not `7.0`). For guaranteed cross-language
> parity prefer integers and strings; the live integration test asserts
> `server_hash == local_hash` end-to-end.

## CLI — offline audit

The package installs an `anchor` command for auditors who hold a CSV export of records
and want to confirm it matches what was anchored on-chain.

```bash
# Fully offline: recompute the root and compare with the on-chain root.
anchor verify --file records.csv --expected-root <root from the blockchain>

# Or let the tool fetch the anchored root from the API by batch id.
anchor verify --file records.csv --api http://host:5001 --domain audit --batch-id audit-...

# Just print the Merkle root of a CSV.
anchor merkle --file records.csv

# Hash a single record.
anchor hash --id ID --timestamp 2026-06-06T00:00:00Z --source app --payload '{"k":"v"}'
```

`verify` exits `0` when **VALID** and `2` when **CORRUPTED**, so it drops into CI/cron.

CSV columns: `id,timestamp,source,payload` (payload is a JSON object string); optional
`hash_fields` (JSON array or comma-separated). Row order must match the anchored batch
(the API returns records in batch order).

## Development

```bash
cd sdk/python
python -m unittest discover -s tests -v          # unit tests (no network)
ANCHOR_BASE_URL=http://localhost:5001 \
  python -m unittest tests.integration_test -v   # live test against a running API
```
