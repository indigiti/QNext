from __future__ import annotations

from dataclasses import asdict, is_dataclass
import json
import os
from pathlib import Path
from typing import Any


class JsonlAppendStore:
    """Append-only durable store with fsync and duplicate-key rejection."""

    def __init__(self, path: str | Path, *, key_field: str):
        self.path = Path(path)
        self.key_field = key_field
        self.path.parent.mkdir(parents=True, exist_ok=True)

    def _mapping(self, value: Any) -> dict[str, Any]:
        if is_dataclass(value):
            return asdict(value)
        if isinstance(value, dict):
            return dict(value)
        raise TypeError("value must be a dataclass or dict")

    def keys(self) -> set[str]:
        if not self.path.exists():
            return set()
        found: set[str] = set()
        with self.path.open("r", encoding="utf-8") as handle:
            for line in handle:
                line = line.strip()
                if not line:
                    continue
                row = json.loads(line)
                found.add(str(row[self.key_field]))
        return found

    def append(self, value: Any) -> None:
        row = self._mapping(value)
        key = str(row[self.key_field])
        if key in self.keys():
            raise ValueError(f"immutable record already exists: {key}")
        payload = json.dumps(row, sort_keys=True, separators=(",", ":"), ensure_ascii=False) + "\n"
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(payload)
            handle.flush()
            os.fsync(handle.fileno())

    def read_all(self) -> list[dict[str, Any]]:
        if not self.path.exists():
            return []
        rows: list[dict[str, Any]] = []
        with self.path.open("r", encoding="utf-8") as handle:
            for line in handle:
                if line.strip():
                    rows.append(json.loads(line))
        return rows
