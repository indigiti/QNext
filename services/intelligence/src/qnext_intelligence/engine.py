from __future__ import annotations

from pathlib import Path
from typing import Sequence

from .domain import Bar, FeatureVector, Outcome, Prediction
from .features import build_feature_vector
from .model import predict
from .outcomes import evaluate_outcome
from .storage import JsonlAppendStore


class IntelligenceEngine:
    def __init__(self, storage_root: str | Path):
        root = Path(storage_root)
        self.feature_store = JsonlAppendStore(root / "features.jsonl", key_field="snapshot_hash")
        self.prediction_store = JsonlAppendStore(root / "predictions.jsonl", key_field="prediction_id")
        self.outcome_store = JsonlAppendStore(root / "outcomes.jsonl", key_field="prediction_id")

    def infer(self, bars: Sequence[Bar], *, lookback: int = 20, horizon_bars: int = 3) -> tuple[FeatureVector, Prediction]:
        vector = build_feature_vector(bars, lookback=lookback)
        prediction = predict(vector, horizon_bars=horizon_bars)
        self.feature_store.append(vector)
        self.prediction_store.append(prediction)
        return vector, prediction

    def attribute(self, prediction: Prediction, bars: Sequence[Bar]) -> Outcome:
        outcome = evaluate_outcome(prediction, bars)
        self.outcome_store.append(outcome)
        return outcome
