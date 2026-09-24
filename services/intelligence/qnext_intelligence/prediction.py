from __future__ import annotations

from collections.abc import Callable, Mapping

from .domain import FeatureVector, ModelManifest, Prediction, stable_hash

ModelFn = Callable[[Mapping[str, float]], Mapping[str, float]]
USABLE_QUALITY = {"GOOD", "RECOVERED"}


def _normalize_probabilities(raw: Mapping[str, float]) -> dict[str, float]:
    allowed = {"UP", "DOWN", "FLAT"}
    values = {key.upper(): float(value) for key, value in raw.items()}
    if not values or not set(values) <= allowed:
        raise ValueError("probabilities must contain only UP/DOWN/FLAT")
    if any(value < 0.0 for value in values.values()):
        raise ValueError("probabilities cannot be negative")
    total = sum(values.values())
    if total <= 0.0:
        raise ValueError("probabilities must have positive mass")
    return {key: value / total for key, value in sorted(values.items())}


def predict(
    features: FeatureVector,
    manifest: ModelManifest,
    model: ModelFn,
    *,
    horizon_bars: int,
    regime: str = "UNSPECIFIED",
) -> Prediction:
    if horizon_bars < 1:
        raise ValueError("horizon_bars must be >= 1")
    manifest.validate_for_inference(features.as_of_time_ms, features.feature_set_version)

    quality = features.data_quality.upper()
    if quality not in USABLE_QUALITY:
        probabilities: dict[str, float] = {}
        direction = "NONE"
        state = "WITHHELD"
    else:
        probabilities = _normalize_probabilities(model(features.features))
        direction = max(probabilities.items(), key=lambda item: (item[1], item[0]))[0]
        state = "IMMUTABLE"

    decision_material = {
        "feature_snapshot_hash": features.snapshot_hash,
        "model_name": manifest.model_name,
        "model_version": manifest.model_version,
        "model_hash": manifest.model_hash,
        "horizon_bars": horizon_bars,
        "regime": regime,
        "probabilities": probabilities,
        "data_quality": quality,
        "state": state,
    }
    decision_context_hash = stable_hash(decision_material)
    prediction_id = stable_hash(
        {
            "instrument_id": features.instrument_id,
            "timeframe": features.timeframe,
            "prediction_time_ms": features.as_of_time_ms,
            "decision_context_hash": decision_context_hash,
        }
    )[:32]

    return Prediction(
        prediction_id=prediction_id,
        instrument_id=features.instrument_id,
        timeframe=features.timeframe,
        prediction_time_ms=features.as_of_time_ms,
        feature_set_version=features.feature_set_version,
        feature_snapshot_hash=features.snapshot_hash,
        model_name=manifest.model_name,
        model_version=manifest.model_version,
        model_hash=manifest.model_hash,
        regime=regime,
        direction=direction,
        probabilities=probabilities,
        horizon_bars=horizon_bars,
        data_quality=quality,
        state=state,
        decision_context_hash=decision_context_hash,
    )
