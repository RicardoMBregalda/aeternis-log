"""Exceptions raised by the anchor SDK."""
from __future__ import annotations


class AnchorError(Exception):
    """Base class for all SDK errors."""


class APIError(AnchorError):
    """Raised for non-2xx responses that are not retried (4xx, or 5xx after
    retries are exhausted)."""

    def __init__(self, status_code: int, body: str) -> None:
        self.status_code = status_code
        self.body = body
        super().__init__(f"anchor: api error {status_code}: {body}")


class HashMismatchError(AnchorError):
    """Raised when the server-returned hash does not match the hash computed
    locally — i.e. the server did not store what the client sent."""

    def __init__(self, server_hash: str, local_hash: str) -> None:
        self.server_hash = server_hash
        self.local_hash = local_hash
        super().__init__(
            f"anchor: server hash {server_hash!r} does not match local hash {local_hash!r}"
        )
