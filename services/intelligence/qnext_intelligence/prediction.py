from __future__ import annotations

import math
from pathlib import Path
from typing import Mapping

from .lifecycle import ModelRegistry
from .models import FeatureSnapshot, Prediction, VALID_DIRECTIONS
from .storage import AppendOnlyJsonlStore, stable_hash


class PredictionError(ValueError):
    pass


class PredictionService:
    def __init__(self, root: str | Path, registry: ModelRegistry) -> None:
        self._store = AppendOnlyJsonlStore(Path(root) / "predictions" / "predictions.jsonl", "prediction_id")
        self._registry = registry

    def create(
        self,
        *,
        snapshot: FeatureSnapshot,
        model_id: str,
        prediction_time_ms: int,
        direction: str,
        probabilities: Mapping[str, float],
        horizon_bars: int,
        regime: str = "UNSPECIFIED",
    ) -> Prediction:
        if self._registry.state(model_id) != "PRODUCTION":
            raise PredictionError("predictions may only use an explicitly promoted production model")
        manifest = self._registry.manifest(model_id)
        assert manifest is not None
        if manifest["feature_set_version"] != snapshot.feature_set_version:
            raise PredictionError("feature set version does not match model manifest")
        if prediction_time_ms < snapshot.as_of_time_ms:
            raise PredictionError("prediction time cannot precede feature snapshot as-of time")
        if horizon_bars <= 0:
            raise PredictionError("horizon_bars must be positive")
        if direction not in VALID_DIRECTIONS:
            raise PredictionError(f"unsupported direction {direction!r}")
        normalized = {str(key): float(value) for key, value in probabilities.items()}
        if set(normalized) != VALID_DIRECTIONS:
            raise PredictionError("probabilities must contain exactly UP, DOWN and FLAT")
        if any((not math.isfinite(value)) or value < 0 or value > 1 for value in normalized.values()):
            raise PredictionError("probabilities must be finite values in [0, 1]")
        if abs(sum(normalized.values()) - 1.0) > 1e-9:
            raise PredictionError("probabilities must sum to 1")
        decision_context = {
            "feature_snapshot_hash": snapshot.snapshot_hash,
            "model_id": model_id,
            "model_hash": manifest["model_hash"],
            "prediction_time_ms": prediction_time_ms,
            "horizon_bars": horizon_bars,
            "regime": regime,
        }
        decision_context_hash = stable_hash(decision_context)
        identity = dict(decision_context)
        identity.update({"instrument_id": snapshot.instrument_id, "timeframe": snapshot.timeframe})
        prediction_id = stable_hash(identity)
        prediction = Prediction(
            prediction_id=prediction_id,
            instrument_id=snapshot.instrument_id,
            timeframe=snapshot.timeframe,
            prediction_time_ms=prediction_time_ms,
            feature_set_version=snapshot.feature_set_version,
            feature_snapshot_hash=snapshot.snapshot_hash,
            model_name=str(manifest["model_name"]),
            model_version=str(manifest["model_version"]),
            model_hash=str(manifest["model_hash"]),
            regime=regime,
            direction=direction,
            probabilities=normalized,
            horizon_bars=horizon_bars,
            data_quality=snapshot.data_quality,
            state="FINAL",
            decision_context_hash=decision_context_hash,
        )
        self._store.append(prediction.to_record())
        return prediction

    def get(self, prediction_id: str) -> dict[str, object] | None:
        return self._store.get(prediction_id)
