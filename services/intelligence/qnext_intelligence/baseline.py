from __future__ import annotations

import math
from collections.abc import Mapping

from .domain import FeatureVector, ModelManifest, stable_hash


BASELINE_MODEL_NAME = "qnext-directional-baseline"
BASELINE_MODEL_VERSION = "1"
BASELINE_ALGORITHM = "deterministic-rule-v1"


def classify_regime(features: FeatureVector) -> str:
    volatility = abs(float(features.features.get("volatility_window", 0.0)))
    distance = float(features.features.get("close_vs_sma", 0.0))
    if abs(distance) <= max(volatility, 0.00025):
        return "RANGE"
    return "TREND_UP" if distance > 0 else "TREND_DOWN"


def baseline_probabilities(values: Mapping[str, float]) -> Mapping[str, float]:
    signal = (
        4.0 * float(values.get("return_1", 0.0))
        + 2.0 * float(values.get("close_vs_sma", 0.0))
        + float(values.get("return_window", 0.0))
    )
    strength = math.tanh(abs(signal) * 50.0)
    directional_mass = 0.45 + (0.4 * strength)
    flat = 1.0 - directional_mass

    if signal > 0:
        up = directional_mass
        down = 0.0
    elif signal < 0:
        up = 0.0
        down = directional_mass
    else:
        up = directional_mass / 2.0
        down = directional_mass / 2.0
    return {"UP": up, "DOWN": down, "FLAT": flat}


def baseline_manifest(feature_set_version: str, inference_time_ms: int) -> ModelManifest:
    if inference_time_ms <= 1:
        raise ValueError("inference time must be positive")
    definition = {
        "name": BASELINE_MODEL_NAME,
        "version": BASELINE_MODEL_VERSION,
        "algorithm": BASELINE_ALGORITHM,
        "feature_set_version": feature_set_version,
    }
    return ModelManifest(
        model_name=BASELINE_MODEL_NAME,
        model_version=BASELINE_MODEL_VERSION,
        lifecycle_state="CANDIDATE",
        feature_set_version=feature_set_version,
        dataset_hash=stable_hash({"type": "rule-based-baseline", "version": 1}),
        model_hash=stable_hash(definition),
        algorithm=BASELINE_ALGORITHM,
        training_window_start_ms=0,
        training_window_end_ms=1,
        created_at_ms=1,
    )
