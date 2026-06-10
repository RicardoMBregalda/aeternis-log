# Guide — Compliance & Audit Trails (LGPD / GDPR / SOX / HIPAA)

Regulations require organizations to prove that audit logs — access events, financial
transactions, consent changes — **have not been altered after the fact**. A database
alone cannot prove this: whoever controls the database can rewrite history.

This platform solves it with the *Tamper-Evident Data Anchoring* pattern: every record
gets a SHA-256 content hash, records are grouped into a Merkle tree, and the root of
each batch is written to a permissioned Hyperledger Fabric ledger. An auditor can then
**recompute the Merkle root from the data and compare it with the on-chain root** — any
change to any record breaks the match, and the comparison does not require trusting the
operator.

## What you get for compliance

| Requirement | How the platform satisfies it |
|---|---|
| Integrity of audit logs | SHA-256 per record + Merkle root anchored on-chain; tamper is mathematically detectable |
| Independent verifiability | Recompute hashes/roots locally (Go/Python SDK, `aeternislog` CLI) — trustless |
| Immutability of the trail | Soft delete preserves the record and its anchor; the proof never disappears |
| Tenant data isolation | Records scoped per tenant; optionally a dedicated Fabric channel per tenant (ledger-level) |
| Auditability over time | Each batch keeps its tx id; the chain of anchors is append-only |
| Access control | API-key auth per tenant; key never leaves the client when verifying |

## 1. Record each auditable event

Send one record per event into a domain (e.g. `access-logs`, `transactions`,
`consent`). The payload is free-form JSON.

```python
from aeternislog import Client
c = Client("https://aeternislog.example.com", api_key="acme-key")

c.create_record(
    "access-logs",
    source="auth-service",
    payload={"actor": "user-42", "action": "read", "resource": "patient/777", "ip": "10.0.0.9"},
)
```

`create_record` computes the hash locally and verifies the server stored the same hash,
so the client has end-to-end assurance from the moment of writing.

## 2. Anchor batches on the blockchain

Anchoring happens automatically on a timer, or on demand:

```python
batch = c.batch_records("access-logs")   # Merkle root committed to Fabric
print(batch.tx_id, batch.merkle_root)
```

In the multi-org production network, the anchor requires endorsement from a **majority
of independent organizations (2 of 3)** — so no single party can forge the proof.

## 3. An auditor verifies — independently

Server-side check (convenience):

```python
result = c.verify_batch("access-logs", batch.batch_id)
assert result.is_valid            # 200 VALID, or 409 CORRUPTED
```

Fully offline check (trustless) — the auditor exports the records to a CSV and obtains
the anchored root from the ledger, then:

```bash
aeternislog verify --file access-logs-2026-06.csv --expected-root <on-chain root>
# exit 0 = VALID, exit 2 = CORRUPTED
```

Because the auditor recomputes the root themselves, a passing result is evidence the
data matches what was anchored — even if the operator is untrusted.

## 4. Deletion without losing the proof (right to erasure)

LGPD/GDPR grant a right to erasure. Soft delete hides a record from active reads while
keeping the document and its on-chain anchor intact, so prior integrity proofs remain
valid. To honor erasure of personal data while keeping the proof, anchor a **hash of
the sensitive fields** (see `hash_fields`) and store the personal data separately:
deleting the data elsewhere does not change the anchored hash.

```python
c.create_record(
    "consent", source="portal",
    payload={"subject_id": "abc", "consent": "granted", "pii_digest": "<hash>"},
    hash_fields=["subject_id", "consent", "pii_digest"],  # only these feed the integrity hash
)
```

## 5. Isolation between tenants/customers

Each tenant is resolved from its API key and only ever sees its own records. For the
strongest isolation, give a tenant its own Fabric channel (a separate ledger) via
`fabric.tenant_channels` — their anchors live on a ledger other tenants cannot read.

## Mapping to common controls

- **GDPR Art. 5(1)(f) / 32 (integrity)** — tamper-evident anchoring + independent verification.
- **LGPD Art. 37 / 46** — provable record of processing operations and security measures.
- **SOX / ISO 27001 change control** — anchor configuration/transaction changes; detect retroactive edits.
- **HIPAA audit controls (§164.312(b))** — immutable, verifiable access logs.

> This is engineering guidance, not legal advice — confirm the controls with your DPO/compliance team.

See also the [Sandbox Quickstart](sandbox-quickstart.md) and the SDK READMEs
([Go](../../sdk/go/README.md), [Python](../../sdk/python/README.md)).
