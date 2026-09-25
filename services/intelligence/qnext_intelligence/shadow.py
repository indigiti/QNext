from __future__ import annotations

from dataclasses import asdict, dataclass

from .domain import Bar, FeatureVector, ModelManifest, Outcome, Prediction, stable_hash
from .outcomes import evaluate_outcome
from .prediction import ModelFn, predict


@dataclass(frozen=True, slots=True)
class ShadowPair:
    shadow_id: str
    production: Prediction
    candidate: Prediction

    def to_record(self) -> dict:
        return {
            "shadow_id": self.shadow_id,
            "production": self.production.to_record(),
            "candidate": self.candidate.to_record(),
        }


@dataclass(frozen=True, slots=True)
class ShadowComparison:
    shadow_id: str
    production_outcome: Outcome
    candidate_outcome: Outcome
    production_correct: bool
    candidate_correct: bool
    candidate_correctness_delta: int

    def to_record(self) -> dict:
        return asdict(self)


def run_shadow(
    features: FeatureVector,
    *,
    production_manifest: ModelManifest,
    production_model: ModelFn,
    candidate_manifest: ModelManifest,
    candidate_model: ModelFn,
    horizon_bars: int,
    regime: str = "UNSPECIFIED",
    strategy_version: str = "UNSPECIFIED",
) -> ShadowPair:
    if production_manifest.lifecycle_state != "PRODUCTION":
        raise ValueError("shadow production manifest must be PRODUCTION")
    if candidate_manifest.lifecycle_state != "CANDIDATE":
        raise ValueError("shadow candidate manifest must be CANDIDATE")

    production = predict(
        features,
        production_manifest,
        production_model,
        horizon_bars=horizon_bars,
        regime=regime,
        strategy_version=strategy_version,
    )
    candidate = predict(
        features,
        candidate_manifest,
        candidate_model,
        horizon_bars=horizon_bars,
        regime=regime,
        strategy_version=strategy_version,
    )
    shadow_id = stable_hash(
        {
            "production_prediction_id": production.prediction_id,
            "candidate_prediction_id": candidate.prediction_id,
            "feature_snapshot_hash": features.snapshot_hash,
        }
    )[:32]
    return ShadowPair(shadow_id=shadow_id, production=production, candidate=candidate)


def evaluate_shadow(
    pair: ShadowPair,
    *,
    origin_close: float,
    future_bars: list[Bar],
) -> ShadowComparison:
    production_outcome = evaluate_outcome(
        pair.production,
        origin_close=origin_close,
        future_bars=future_bars,
    )
    candidate_outcome = evaluate_outcome(
        pair.candidate,
        origin_close=origin_close,
        future_bars=future_bars,
    )
    return ShadowComparison(
        shadow_id=pair.shadow_id,
        production_outcome=production_outcome,
        candidate_outcome=candidate_outcome,
        production_correct=production_outcome.correct,
        candidate_correct=candidate_outcome.correct,
        candidate_correctness_delta=int(candidate_outcome.correct) - int(production_outcome.correct),
    )
