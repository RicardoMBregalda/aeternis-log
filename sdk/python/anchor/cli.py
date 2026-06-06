"""``anchor`` command-line tool.

Offline integrity verification for auditors: given a CSV export of records,
recompute the Merkle root locally and compare it with the root anchored on the
blockchain — without trusting the API to do the recomputation.

    anchor merkle  --file records.csv
    anchor verify  --file records.csv --expected-root <on-chain root>
    anchor verify  --file records.csv --api http://host:5001 --domain audit \\
                   --batch-id audit-... [--key <api-key>]
    anchor hash    --id ID --timestamp TS --source SRC --payload '{"k":"v"}'

CSV columns: id, timestamp, source, payload (a JSON object string); optional
hash_fields (JSON array or comma-separated). Row order must match the anchored
batch (the API returns records in batch order).
"""
from __future__ import annotations

import argparse
import csv
import json
import sys
from typing import List, Optional, Sequence

from .client import Client
from .errors import AnchorError
from .record import Record, merkle_root


def _load_csv(path: str) -> List[Record]:
    with open(path, newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        required = {"id", "timestamp", "source", "payload"}
        missing = required - set(reader.fieldnames or [])
        if missing:
            raise SystemExit(f"error: CSV is missing required columns: {sorted(missing)}")

        records: List[Record] = []
        for line, row in enumerate(reader, start=2):
            raw_payload = (row.get("payload") or "").strip()
            try:
                payload = json.loads(raw_payload) if raw_payload else {}
            except json.JSONDecodeError as e:
                raise SystemExit(f"error: row {line}: invalid payload JSON: {e}")

            hash_fields: Optional[List[str]] = None
            raw_hf = (row.get("hash_fields") or "").strip()
            if raw_hf:
                try:
                    hash_fields = json.loads(raw_hf)
                except json.JSONDecodeError:
                    hash_fields = [s.strip() for s in raw_hf.split(",") if s.strip()]

            records.append(
                Record(
                    id=row["id"],
                    timestamp=row["timestamp"],
                    source=row["source"],
                    payload=payload,
                    hash_fields=hash_fields,
                )
            )
    return records


def _cmd_merkle(args: argparse.Namespace) -> int:
    print(merkle_root(_load_csv(args.file)))
    return 0


def _cmd_hash(args: argparse.Namespace) -> int:
    payload = json.loads(args.payload) if args.payload else {}
    rec = Record(
        id=args.id,
        timestamp=args.timestamp,
        source=args.source,
        payload=payload,
        hash_fields=args.hash_field or None,
    )
    print(rec.compute_hash())
    return 0


def _cmd_verify(args: argparse.Namespace) -> int:
    records = _load_csv(args.file)
    local_root = merkle_root(records)

    expected = args.expected_root
    if not expected and args.batch_id:
        if not args.domain:
            raise SystemExit("error: --batch-id requires --domain")
        client = Client(args.api, api_key=args.key)
        try:
            expected = client.verify_batch(args.domain, args.batch_id).original_merkle_root
        except AnchorError as e:
            raise SystemExit(f"error: could not fetch anchored root: {e}")
    if not expected:
        raise SystemExit("error: provide --expected-root, or --api/--domain/--batch-id")

    ok = local_root == expected
    print(f"records:       {len(records)}")
    print(f"local root:    {local_root}")
    print(f"anchored root: {expected}")
    print(f"integrity:     {'VALID' if ok else 'CORRUPTED'}")
    return 0 if ok else 2


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="anchor", description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="command", required=True)

    p_merkle = sub.add_parser("merkle", help="compute the Merkle root of a CSV (offline)")
    p_merkle.add_argument("--file", required=True, help="path to the records CSV")
    p_merkle.set_defaults(func=_cmd_merkle)

    p_verify = sub.add_parser("verify", help="recompute a CSV's root and compare with the anchored root")
    p_verify.add_argument("--file", required=True, help="path to the records CSV")
    p_verify.add_argument("--expected-root", default="", help="anchored Merkle root (fully offline mode)")
    p_verify.add_argument("--api", default="http://localhost:5001", help="API base URL (to fetch the anchored root)")
    p_verify.add_argument("--domain", default="", help="record domain (with --batch-id)")
    p_verify.add_argument("--batch-id", default="", help="anchored batch id (fetch its root via the API)")
    p_verify.add_argument("--key", default="", help="API key")
    p_verify.set_defaults(func=_cmd_verify)

    p_hash = sub.add_parser("hash", help="compute the integrity hash of a single record")
    p_hash.add_argument("--id", required=True)
    p_hash.add_argument("--timestamp", required=True)
    p_hash.add_argument("--source", required=True)
    p_hash.add_argument("--payload", default="{}", help="payload as a JSON object")
    p_hash.add_argument("--hash-field", action="append", help="restrict the hash to this payload key (repeatable)")
    p_hash.set_defaults(func=_cmd_hash)

    return parser


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = build_parser().parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
