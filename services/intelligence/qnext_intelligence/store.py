from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any, Iterator

from .domain import canonical_json


class ImmutableJSONLStore:
    """Append-only JSONL store with duplicate-identity rejection.

    This is intentionally simple for the no-database QNext baseline. It provides
    process-local serialization and durable append semantics; deployment should
    use a single writer per file.
    """

    def __init__(self, path: str | Path, *, identity_field: str) -> None:
        self.path = Path(path)
        self.identity_field = identity_field

    def _iter_records(self) -> Iterator[dict[str, Any]]:
        if not self.path.exists():
            return
        with self.path.open("r", encoding="utf-8") as handle:
            for line_number, line in enumerate(handle, start=1):
                if not line.strip():
                    continue
                try:
                    yield json.loads(line)
                except json.JSONDecodeError as exc:
                    raise ValueError(f"corrupt JSONL at line {line_number}") from exc
    def read_all(self) -> list[dict[str, Any]]:
        return list(self._iter_records())

    def append(self, record: dict[str, Any]) -> None:
        identity = record.get(self.identity_field)
        if not identity:
            raise ValueError(f"missing identity field: {self.identity_field}")
        # Keep scanning after the first match: malformed JSON later in the
        # file must still fail before any duplicate error or durable append.
        conflict: Exception | None = None
        for existing in self._iter_records():
            if conflict is not None:
                continue
            try:
                if existing.get(self.identity_field) == identity:
                    if existing == record:
                        conflict = ValueError("immutable record already exists")
                    else:
                        conflict = ValueError("identity collision with different immutable content")
            except AttributeError as exc:
                conflict = exc
        if conflict is not None:
            raise conflict

        self.path.parent.mkdir(parents=True, exist_ok=True)
        payload = canonical_json(record) + "\n"
        fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o640)
        try:
            os.write(fd, payload.encode("utf-8"))
            os.fsync(fd)
        finally:
            os.close(fd)
