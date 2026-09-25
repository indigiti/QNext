from __future__ import annotations

from dataclasses import asdict, dataclass
import hashlib
import json
from typing import Mapping


def canonical_json(value: object) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def stable_hash(value: object) -> str:
    return hashlib.sha256(canonical_json(value).encode("utf-8")).hexdigest()


@dataclass(frozen=True, slots=True)
class Bar:
    instrument_id: str
    timeframe: str
    open_time_ms: int
    close_time_ms: int
    open: float
    high: float
    low: float
    close: float
    volume: float = 0.0
    final: bool = True
    revision: int = 0
    quality: str = "GOOD"
    authority_provider: str = ""

    def validate(self) -> None:
        if self.close_time_ms <= self.open_time_ms:
            raise ValueError("bar close_time_ms must be after open_time_ms")
        if self.high < max(self.open, self.close) or self.low > min(self.open, self.close):
            raise ValueError("bar OHLC bounds are inconsistent")
        if self.high < self.low:
            raise ValueError("bar high must be >= low")


@dataclass(frozen=True, slots=True)
class FeatureVector:
    feature_set_version: str
    instrument_id: str
    timeframe: str
    as_of_time_ms: int
    features: Mapping[str, float]
    snapshot_hash: str
    data_quality: str
    authority_provider: str
    calendar_version: str
    session: str
    configuration_hash: str

    def to_record(self) -> dict:
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
    promoted_at_ms: int = 0
    retired_at_ms: int = 0

    def validate_for_inference(self, as_of_time_ms: int, feature_set_version: str) -> None:
        if self.lifecycle_state not in {"CANDIDATE", "PRODUCTION"}:
            raise ValueError("model is not eligible for inference")
        if self.feature_set_version != feature_set_version:
            raise ValueError("feature set version mismatch")
        if self.training_window_end_ms >= as_of_time_ms:
            raise ValueError("model training window overlaps or extends beyond inference time")


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
    probabilities: Mapping[str, float]
    horizon_bars: int
    data_quality: str
    state: str
    authority_provider: str
    calendar_version: str
    session: str
    strategy_version: str
    configuration_hash: str
    decision_context_hash: str

    def to_record(self) -> dict:
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

    def to_record(self) -> dict:
        return asdict(self)
