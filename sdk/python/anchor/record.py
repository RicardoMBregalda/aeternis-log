"""Local, trustless record hashing and Merkle verification.

This mirrors the server's algorithm exactly, so an auditor can verify integrity
*without trusting the API*:

    hash(record) = SHA-256(id + timestamp + source + canonical(payload))

``canonical`` reproduces Go's ``encoding/json`` output (keys sorted, compact
separators, UTF-8 with HTML-sensitive characters escaped) so a hash computed here
matches the one produced by the Go server and the Go SDK byte-for-byte.

Payload type note: the server decodes JSON numbers as 64-bit floats, so this module
formats integer-valued numbers without a decimal point (``7`` not ``7.0``) to match.
For guaranteed cross-language parity, prefer integers and strings in payloads; the
live integration test asserts ``server_hash == local_hash`` end-to-end.
"""
from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, List, Mapping, Optional, Sequence

# Characters Go's encoding/json escapes inside strings that Python's json does not.
_GO_STRING_ESCAPE = {
    ord("<"): "\\u003c",
    ord(">"): "\\u003e",
    ord("&"): "\\u0026",
    0x2028: "\\u2028",
    0x2029: "\\u2029",
}


def _encode_string(s: str) -> str:
    """JSON-encode a string the way Go's encoding/json does."""
    return json.dumps(s, ensure_ascii=False).translate(_GO_STRING_ESCAPE)


def _encode_float(f: float) -> str:
    if f != f or f in (float("inf"), float("-inf")):
        raise ValueError("NaN and Infinity are not valid in canonical JSON")
    # Go marshals integer-valued float64 without a fractional part (7.0 -> "7").
    if f == int(f) and abs(f) < 1e21:
        return str(int(f))
    return repr(f)


def canonical(value: Any) -> str:
    """Serialize ``value`` to match Go's ``encoding/json.Marshal`` byte-for-byte."""
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, str):
        return _encode_string(value)
    if isinstance(value, int):  # bool is handled above
        return str(value)
    if isinstance(value, float):
        return _encode_float(value)
    if isinstance(value, Mapping):
        items = sorted(value.items(), key=lambda kv: str(kv[0]))
        return "{" + ",".join(_encode_string(str(k)) + ":" + canonical(v) for k, v in items) + "}"
    if isinstance(value, (list, tuple)):
        return "[" + ",".join(canonical(v) for v in value) + "]"
    raise TypeError(f"unsupported payload type: {type(value).__name__}")


@dataclass
class Record:
    """A record as created or returned by the API."""

    source: str = ""
    payload: dict = field(default_factory=dict)
    domain: str = ""
    id: str = ""
    timestamp: str = ""
    hash_fields: Optional[Sequence[str]] = None
    hash: str = ""
    batch_id: str = ""
    merkle_root: str = ""

    def compute_hash(self) -> str:
        """Recompute this record's integrity hash locally and independently."""
        payload: Mapping[str, Any] = self.payload
        if self.hash_fields:
            payload = {k: self.payload[k] for k in self.hash_fields if k in self.payload}
        content = self.id + self.timestamp + self.source + canonical(payload)
        return hashlib.sha256(content.encode("utf-8")).hexdigest()

    def verify(self) -> bool:
        """True if the stored hash matches the locally recomputed one."""
        return bool(self.hash) and self.hash == self.compute_hash()

    @classmethod
    def from_api(cls, data: Mapping[str, Any]) -> "Record":
        return cls(
            source=data.get("source", ""),
            payload=data.get("payload", {}) or {},
            domain=data.get("domain", ""),
            id=data.get("id", ""),
            timestamp=data.get("timestamp", ""),
            hash_fields=data.get("hash_fields"),
            hash=data.get("hash", ""),
            batch_id=data.get("batch_id", ""),
            merkle_root=data.get("merkle_root", ""),
        )


def _build_merkle_tree(hashes: Sequence[str]) -> str:
    if not hashes:
        return ""
    level: List[str] = list(hashes)
    while len(level) > 1:
        if len(level) % 2 != 0:
            level.append(level[-1])
        level = [
            hashlib.sha256((level[i] + level[i + 1]).encode("utf-8")).hexdigest()
            for i in range(0, len(level), 2)
        ]
    return level[0]


def merkle_root(records: Sequence[Record]) -> str:
    """Recompute the Merkle root of an ordered set of records, locally."""
    return _build_merkle_tree([r.compute_hash() for r in records])


def verify_records_locally(records: Sequence[Record], expected_root: str) -> bool:
    """True only if every record still hashes into ``expected_root`` (e.g. the
    root anchored on the blockchain). Order must match the anchored batch."""
    return merkle_root(records) == expected_root
