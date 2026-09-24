from __future__ import annotations

from math import exp
from typing import Mapping

from .domain import FeatureVector, Prediction, stable_hash

MODEL_NAME = "qnext-direction-baseline"
MODEL_VERSION = "1.0.0"
HORIZON_BARS = 3

_WEIGHTS: Mapping[str, float] = {
    "return_1": 4.0,
    "return_3": 6.0,
    "return_5": 4.0,
    "ema_5_distance": 8.0,
    "ema_20_distance": 5.0,
    "range_position_20": 1.2,
    "body_pct": 2.5,
    "volatility_20": -1.5,
}


def model_hash() -> str:
    return stable_hash({"name": MODEL_NAME, "version": MODEL_VERSION, "weights": dict(_WEIGHTS)})


def _sigmoid(value: float) -> float:
    if value >= 0:
        z = exp(-value)
        return 1.0 / (1.0 + z)
    z = exp(value)
    return z / (1.0 + z)


def detect_regime(vector: FeatureVector) -> str:
    f = vector.features
    trend = abs(f["ema_20_distance"])
    volatility = f["volatility_20"]
    if volatility >= 0.008:
        return "HIGH_VOLATILITY"
    if trend >= 0.004:
        return "TRENDING"
    return "RANGING"


def predict(vector: FeatureVector, *, horizon_bars: int = HORIZON_BARS) -> Prediction:
    score = sum(_WEIGHTS.get(name, 0.0) * value for name, value in vector.features.items())
    up = _sigmoid(score)
    down = 1.0 - up
    flat = max(0.0, 1.0 - min(1.0, abs(score) * 2.5)) * 0.20
    scale = max(up + down + flat, 1e-12)
    probabilities = {"UP": up / scale, "DOWN": down / scale, "FLAT": flat / scale}
    direction = max(probabilities, key=probabilities.__getitem__)
    regime = detect_regime(vector)
    mh = model_hash()
    decision_context = {
        "feature_snapshot_hash": vector.snapshot_hash,
        "model_hash": mh,
        "regime": regime,
        "horizon_bars": horizon_bars,
        "probabilities": probabilities,
    }
    decision_context_hash = stable_hash(decision_context)
    prediction_id = stable_hash(
        {
            "instrument_id": vector.instrument_id,
            "timeframe": vector.timeframe,
            "prediction_time_ms": vector.as_of_time_ms,
            "feature_snapshot_hash": vector.snapshot_hash,
            "model_hash": mh,
            "horizon_bars": horizon_bars,
        }
    )[:32]
    state = "ACTIVE" if vector.data_quality == "GOOD" else "WITHHELD"
    return Prediction(
        prediction_id=prediction_id,
        instrument_id=vector.instrument_id,
        timeframe=vector.timeframe,
        prediction_time_ms=vector.as_of_time_ms,
        feature_set_version=vector.feature_set_version,
        feature_snapshot_hash=vector.snapshot_hash,
        model_name=MODEL_NAME,
        model_version=MODEL_VERSION,
        model_hash=mh,
        regime=regime,
        direction=direction,
        probabilities=probabilities,
        horizon_bars=horizon_bars,
        data_quality=vector.data_quality,
        state=state,
        decision_context_hash=decision_context_hash,
    )
