from __future__ import annotations

from dataclasses import dataclass
from typing import Mapping

from .storage import stable_hash


VALID_QUALITY = {"GOOD", "RECOVERED", "PARTIAL", "STALE", "DEGRADED", "INVALID"}
VALID_DIRECTIONS = {"UP", "DOWN", "FLAT"}


@dataclass(frozen=True)
class FeaturePoint:
    name: str
    value: float
    source_time_ms: int

    def __post_init__(self) -> None:
        if not self.name.strip():
            raise ValueError("feature name is required")
        if self.source_time_ms <= 0:
            raise ValueError("source_time_ms must be positive")

    def to_record(self) -> dict[str, object]:
        return {"name": self.name, "value": float(self.value), "source_time_ms": self.source_time_ms}


@dataclass(frozen=True)
class FeatureSnapshot:
    feature_set_version: str
    instrument_id: str
    timeframe: str
    as_of_time_ms: int
    features: tuple[FeaturePoint, ...]
    data_quality: str
    snapshot_hash: str

    @classmethod
    def create(
        cls,
        *,
        feature_set_version: str,
        instrument_id: str,
        timeframe: str,
        as_of_time_ms: int,
        features: tuple[FeaturePoint, ...] | list[FeaturePoint],
        data_quality: str = "GOOD",
    ) -> "FeatureSnapshot":
        if not feature_set_version or not instrument_id or not timeframe:
            raise ValueError("feature_set_version, instrument_id and timeframe are required")
        if as_of_time_ms <= 0:
            raise ValueError("as_of_time_ms must be positive")
        if data_quality not in VALID_QUALITY:
            raise ValueError(f"unsupported data quality {data_quality!r}")
        ordered = tuple(sorted(features, key=lambda item: item.name))
        if not ordered:
            raise ValueError("at least one feature is required")
        names = [item.name for item in ordered]
        if len(names) != len(set(names)):
            raise ValueError("feature names must be unique")
        future = [item.name for item in ordered if item.source_time_ms > as_of_time_ms]
        if future:
            raise ValueError(f"future information detected: {', '.join(future)}")
        payload = {
            "feature_set_version": feature_set_version,
            "instrument_id": instrument_id,
            "timeframe": timeframe,
            "as_of_time_ms": as_of_time_ms,
            "features": [item.to_record() for item in ordered],
            "data_quality": data_quality,
        }
        return cls(
            feature_set_version=feature_set_version,
            instrument_id=instrument_id,
            timeframe=timeframe,
            as_of_time_ms=as_of_time_ms,
            features=ordered,
            data_quality=data_quality,
            snapshot_hash=stable_hash(payload),
        )

    def to_record(self) -> dict[str, object]:
        return {
            "feature_set_version": self.feature_set_version,
            "instrument_id": self.instrument_id,
            "timeframe": self.timeframe,
            "as_of_time_ms": self.as_of_time_ms,
            "features": [item.to_record() for item in self.features],
            "snapshot_hash": self.snapshot_hash,
            "data_quality": self.data_quality,
        }


@dataclass(frozen=True)
class ModelManifest:
    model_name: str
    model_version: str
    feature_set_version: str
    dataset_hash: str
    model_hash: str
    algorithm: str
    training_window_start_ms: int
    training_window_end_ms: int
    created_at_ms: int

    def __post_init__(self) -> None:
        required = [self.model_name, self.model_version, self.feature_set_version, self.dataset_hash, self.model_hash, self.algorithm]
        if any(not value for value in required):
            raise ValueError("model manifest string fields are required")
        if self.training_window_start_ms <= 0 or self.training_window_end_ms <= 0 or self.created_at_ms <= 0:
            raise ValueError("model manifest timestamps must be positive")
        if self.training_window_end_ms < self.training_window_start_ms:
            raise ValueError("training window end cannot precede start")

    @property
    def model_id(self) -> str:
        return f"{self.model_name}:{self.model_version}"

    def to_record(self) -> dict[str, object]:
        return {
            "model_id": self.model_id,
            "model_name": self.model_name,
            "model_version": self.model_version,
            "lifecycle_state": "CANDIDATE",
            "feature_set_version": self.feature_set_version,
            "dataset_hash": self.dataset_hash,
            "model_hash": self.model_hash,
            "algorithm": self.algorithm,
            "training_window_start_ms": self.training_window_start_ms,
            "training_window_end_ms": self.training_window_end_ms,
            "created_at_ms": self.created_at_ms,
        }


@dataclass(frozen=True)
class Prediction:
    prediction_id: str
    instrument_id: str
    timeframe: str
    prediction_time_ms: int
    feature_set_version: str
    feature_snapshot_hash: str
    model_name: str
    model_version: str
    model_hash: str
    regime: str
    direction: str
    probabilities: Mapping[str, float]
    horizon_bars: int
    data_quality: str
    state: str
    decision_context_hash: str

    def to_record(self) -> dict[str, object]:
        return {
            "prediction_id": self.prediction_id,
            "instrument_id": self.instrument_id,
            "timeframe": self.timeframe,
            "prediction_time_ms": self.prediction_time_ms,
            "feature_set_version": self.feature_set_version,
            "feature_snapshot_hash": self.feature_snapshot_hash,
            "model_name": self.model_name,
            "model_version": self.model_version,
            "model_hash": self.model_hash,
            "regime": self.regime,
            "direction": self.direction,
            "probabilities": dict(sorted(self.probabilities.items())),
            "horizon_bars": self.horizon_bars,
            "data_quality": self.data_quality,
            "state": self.state,
            "decision_context_hash": self.decision_context_hash,
        }


@dataclass(frozen=True)
class Outcome:
    prediction_id: str
    return_value: float
    mfe: float
    mae: float
    direction_actual: str
    correct: bool
    evaluated_at_ms: int

    def to_record(self) -> dict[str, object]:
        return {
            "prediction_id": self.prediction_id,
            "return_value": self.return_value,
            "mfe": self.mfe,
            "mae": self.mae,
            "direction_actual": self.direction_actual,
            "correct": self.correct,
            "evaluated_at_ms": self.evaluated_at_ms,
        }
