from __future__ import annotations

from pathlib import Path

from .models import FeaturePoint, FeatureSnapshot
from .storage import AppendOnlyJsonlStore


class FeatureStore:
    def __init__(self, root: str | Path) -> None:
        self._store = AppendOnlyJsonlStore(Path(root) / "features" / "snapshots.jsonl", "snapshot_hash")

    def create_snapshot(
        self,
        *,
        feature_set_version: str,
        instrument_id: str,
        timeframe: str,
        as_of_time_ms: int,
        features: list[FeaturePoint] | tuple[FeaturePoint, ...],
        data_quality: str = "GOOD",
    ) -> FeatureSnapshot:
        snapshot = FeatureSnapshot.create(
            feature_set_version=feature_set_version,
            instrument_id=instrument_id,
            timeframe=timeframe,
            as_of_time_ms=as_of_time_ms,
            features=features,
            data_quality=data_quality,
        )
        self._store.append(snapshot.to_record())
        return snapshot

    def get(self, snapshot_hash: str) -> dict[str, object] | None:
        return self._store.get(snapshot_hash)
