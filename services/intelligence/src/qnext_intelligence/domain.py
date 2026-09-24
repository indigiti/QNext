from __future__ import annotations

from dataclasses import asdict, dataclass
from hashlib import sha256
import json
from typing import Any, Mapping


def canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def stable_hash(value: Any) -> str:
    return sha256(canonical_json(value).encode("utf-8")).hexdigest()


@dataclass(frozen=True, slots=True)
class Bar:
    instrument_id: str
    timeframe: str
    time_ms: int
    open: float
    high: float
    low: float
    close: float
    volume: float
    final: bool
    revision: int
    quality: str
    authority_provider: str

    @classmethod
    def from_market_core(
        cls,
        value: Mapping[str, Any],
        *,
        instrument_id: str,
        timeframe: str,
    ) -> "Bar":
        return cls(
            instrument_id=instrument_id,
            timeframe=timeframe,
            time_ms=int(value["time"]),
            open=float(value["open"]),
            high=float(value["high"]),
            low=float(value["low"]),
            close=float(value["close"]),
            volume=float(value.get("volume", 0.0)),
            final=bool(value.get("final", False)),
            revision=int(value.get("revision", 0)),
            quality=str(value.get("quality", "INVALID")),
            authority_provider=str(value.get("authority_provider", "")),
        )


@dataclass(frozen=True, slots=True)
class FeatureVector:
    feature_set_version: str
    instrument_id: str
    timeframe: str
    as_of_time_ms: int
    features: dict[str, float]
    snapshot_hash: str
    data_quality: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True, slots=True)
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
    probabilities: dict[str, float]
    horizon_bars: int
    data_quality: str
    state: str
    decision_context_hash: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True, slots=True)
class Outcome:
    prediction_id: str
    return_value: float
    mfe: float
    mae: float
    direction_actual: str
    correct: bool
    evaluated_at_ms: int

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True, slots=True)
class ModelManifest:
    model_name: str
    model_version: str
    lifecycle_state: str
    feature_set_version: str
    dataset_hash: str
    model_hash: str
    algorithm: str
    training_window_start_ms: int
    training_window_end_ms: int
    created_at_ms: int
    promoted_at_ms: int
    retired_at_ms: int

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)
