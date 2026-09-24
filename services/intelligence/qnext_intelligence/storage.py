from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
from typing import Any, Iterable


class ImmutableRecordError(ValueError):
    """Raised when a caller attempts to mutate an append-only record."""


def canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def stable_hash(value: Any) -> str:
    return hashlib.sha256(canonical_json(value).encode("utf-8")).hexdigest()


class AppendOnlyJsonlStore:
    def __init__(self, path: str | Path, key_field: str) -> None:
        self.path = Path(path)
        self.key_field = key_field
        self.path.parent.mkdir(parents=True, exist_ok=True)

    def records(self) -> Iterable[dict[str, Any]]:
        if not self.path.exists():
            return ()
        items: list[dict[str, Any]] = []
        with self.path.open("r", encoding="utf-8") as handle:
            for line in handle:
                line = line.strip()
                if line:
                    items.append(json.loads(line))
        return tuple(items)

    def get(self, key: str) -> dict[str, Any] | None:
        for record in self.records():
            if str(record.get(self.key_field)) == key:
                return record
        return None

    def append(self, record: dict[str, Any]) -> dict[str, Any]:
        if self.key_field not in record:
            raise ValueError(f"record missing key field {self.key_field!r}")
        key = str(record[self.key_field])
        existing = self.get(key)
        if existing is not None:
            if canonical_json(existing) == canonical_json(record):
                return existing
            raise ImmutableRecordError(f"record {key!r} is immutable")
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(canonical_json(record) + "\n")
            handle.flush()
            os.fsync(handle.fileno())
        return record
