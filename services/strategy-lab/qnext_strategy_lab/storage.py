from __future__ import annotations

import json
from pathlib import Path

from .engine import BacktestResult


class ImmutableRunStore:
    """File-backed immutable storage for Q4 run artifacts."""

    def __init__(self, root: str | Path) -> None:
        self.root = Path(root)

    def write(self, result: BacktestResult) -> Path:
        self.root.mkdir(parents=True, exist_ok=True)
        destination = self.root / f"{result.run_id}.json"
        content = json.dumps(result.summary(), sort_keys=True, indent=2) + "\n"
        if destination.exists():
            existing = destination.read_text(encoding="utf-8")
            if existing != content:
                raise RuntimeError("immutable run id collision")
            return destination
        temp = destination.with_suffix(".json.tmp")
        temp.write_text(content, encoding="utf-8")
        temp.replace(destination)
        return destination
