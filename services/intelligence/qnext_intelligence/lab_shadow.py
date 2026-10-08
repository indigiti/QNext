from __future__ import annotations

from dataclasses import dataclass
import math
from statistics import fmean
from typing import Any, Mapping, Sequence

from .domain import Bar, stable_hash
from .lab_ml import LabMLCandidate
from .lab_recommendation import (
    _direction,
    _model_probability_one,
    _temperature_scale,
)
from .lab_snapshot import LabCurrentFeatureVector
from .learning import (
    CandidateModel,
    EvaluationMetrics,
    LearningExample,
    evaluate_directions,
)

USABLE_QUALITY = {"GOOD", "RECOVERED"}
ACTION_TO_DIRECTION = {"BUY": "UP", "SELL": "DOWN", "NO_TRADE": "FLAT"}
DIRECTION_TO_ACTION = {"UP": "BUY", "DOWN": "SELL", "FLAT": "NO_TRADE"}
EPSILON = 1e-9


@dataclass(frozen=True, slots=True)
class ShadowConfig:
    experiment_id: str
    started_at_ms: int
    horizon_bars: int
    min_samples: int = 30
    max_accuracy_regression: float = 0.05
    max_average_return_regression: float = 0.002
    max_drawdown_slack: float = 0.02
    max_brier: float = 0.35
    min_coverage: float = 0.10
    min_target1_before_invalidation: float = 0.30

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": "QNEXT.INTELLIGENCE.LAB.SHADOW_CONFIG/1",
            "experiment_id": self.experiment_id,
            "started_at_ms": self.started_at_ms,
            "horizon_bars": self.horizon_bars,
            "min_samples": self.min_samples,
            "max_accuracy_regression": self.max_accuracy_regression,
            "max_average_return_regression": self.max_average_return_regression,
            "max_drawdown_slack": self.max_drawdown_slack,
            "max_brier": self.max_brier,
            "min_coverage": self.min_coverage,
            "min_target1_before_invalidation": self.min_target1_before_invalidation,
        }

    @classmethod
    def from_record(cls, record: Mapping[str, Any]) -> "ShadowConfig":
        return cls(
            experiment_id=str(record["experiment_id"]),
            started_at_ms=int(record["started_at_ms"]),
            horizon_bars=int(record["horizon_bars"]),
            min_samples=int(record.get("min_samples", 30)),
            max_accuracy_regression=float(record.get("max_accuracy_regression", 0.05)),
            max_average_return_regression=float(record.get("max_average_return_regression", 0.002)),
            max_drawdown_slack=float(record.get("max_drawdown_slack", 0.02)),
            max_brier=float(record.get("max_brier", 0.35)),
            min_coverage=float(record.get("min_coverage", 0.10)),
            min_target1_before_invalidation=float(
                record.get("min_target1_before_invalidation", 0.30)
            ),
        )


@dataclass(frozen=True, slots=True)
class ShadowObservation:
    observation_id: str
    experiment_id: str
    bar_time_ms: int
    as_of_time_ms: int
    created_at_ms: int
    origin_close: float
    model_algorithm: str
    model_hash: str
    feature_snapshot_hash: str
    features: Mapping[str, float]
    decision: str
    probabilities: Mapping[str, float]
    target1_pct: float | None
    target2_pct: float | None
    invalidation_pct: float | None

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": "QNEXT.INTELLIGENCE.LAB.SHADOW_OBSERVATION/1",
            "observation_id": self.observation_id,
            "experiment_id": self.experiment_id,
            "bar_time_ms": self.bar_time_ms,
            "as_of_time_ms": self.as_of_time_ms,
            "created_at_ms": self.created_at_ms,
            "origin_close": self.origin_close,
            "model_algorithm": self.model_algorithm,
            "model_hash": self.model_hash,
            "feature_snapshot_hash": self.feature_snapshot_hash,
            "features": dict(self.features),
            "decision": self.decision,
            "probabilities": dict(self.probabilities),
            "target1_pct": self.target1_pct,
            "target2_pct": self.target2_pct,
            "invalidation_pct": self.invalidation_pct,
        }

    @classmethod
    def from_record(cls, record: Mapping[str, Any]) -> "ShadowObservation":
        return cls(
            observation_id=str(record["observation_id"]),
            experiment_id=str(record["experiment_id"]),
            bar_time_ms=int(record["bar_time_ms"]),
            as_of_time_ms=int(record["as_of_time_ms"]),
            created_at_ms=int(record["created_at_ms"]),
            origin_close=float(record["origin_close"]),
            model_algorithm=str(record["model_algorithm"]),
            model_hash=str(record["model_hash"]),
            feature_snapshot_hash=str(record["feature_snapshot_hash"]),
            features={str(key): float(value) for key, value in dict(record["features"]).items()},
            decision=str(record["decision"]),
            probabilities={
                str(key): float(value)
                for key, value in dict(record["probabilities"]).items()
            },
            target1_pct=_optional_float(record.get("target1_pct")),
            target2_pct=_optional_float(record.get("target2_pct")),
            invalidation_pct=_optional_float(record.get("invalidation_pct")),
        )


@dataclass(frozen=True, slots=True)
class ShadowOutcome:
    observation_id: str
    experiment_id: str
    evaluated_at_ms: int
    actual_direction: str
    terminal_return: float
    mfe: float
    mae: float
    correct: bool
    strategy_return: float
    brier: float
    target1_before_invalidation: bool | None
    target2_before_invalidation: bool | None
    invalidation_before_target1: bool | None

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": "QNEXT.INTELLIGENCE.LAB.SHADOW_OUTCOME/1",
            "observation_id": self.observation_id,
            "experiment_id": self.experiment_id,
            "evaluated_at_ms": self.evaluated_at_ms,
            "actual_direction": self.actual_direction,
            "terminal_return": self.terminal_return,
            "mfe": self.mfe,
            "mae": self.mae,
            "correct": self.correct,
            "strategy_return": self.strategy_return,
            "brier": self.brier,
            "target1_before_invalidation": self.target1_before_invalidation,
            "target2_before_invalidation": self.target2_before_invalidation,
            "invalidation_before_target1": self.invalidation_before_target1,
        }

    @classmethod
    def from_record(cls, record: Mapping[str, Any]) -> "ShadowOutcome":
        return cls(
            observation_id=str(record["observation_id"]),
            experiment_id=str(record["experiment_id"]),
            evaluated_at_ms=int(record["evaluated_at_ms"]),
            actual_direction=str(record["actual_direction"]),
            terminal_return=float(record["terminal_return"]),
            mfe=float(record["mfe"]),
            mae=float(record["mae"]),
            correct=bool(record["correct"]),
            strategy_return=float(record["strategy_return"]),
            brier=float(record["brier"]),
            target1_before_invalidation=_optional_bool(
                record.get("target1_before_invalidation")
            ),
            target2_before_invalidation=_optional_bool(
                record.get("target2_before_invalidation")
            ),
            invalidation_before_target1=_optional_bool(
                record.get("invalidation_before_target1")
            ),
        )


@dataclass(frozen=True, slots=True)
class ShadowSummary:
    completed_samples: int
    pending_samples: int
    metrics: EvaluationMetrics
    average_brier: float
    target1_samples: int
    target1_before_invalidation: float
    target2_before_invalidation: float
    invalidation_before_target1: float

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": "QNEXT.INTELLIGENCE.LAB.SHADOW_SUMMARY/1",
            "completed_samples": self.completed_samples,
            "pending_samples": self.pending_samples,
            "metrics": self.metrics.to_record(),
            "average_brier": self.average_brier,
            "target1_samples": self.target1_samples,
            "target1_before_invalidation": self.target1_before_invalidation,
            "target2_before_invalidation": self.target2_before_invalidation,
            "invalidation_before_target1": self.invalidation_before_target1,
        }


def score_shadow_observation(
    *,
    experiment_id: str,
    current: LabCurrentFeatureVector,
    selected_model: CandidateModel | LabMLCandidate,
    recommendation_policy: Mapping[str, Any],
    created_at_ms: int,
) -> ShadowObservation:
    calibration = recommendation_policy.get("calibration")
    if not isinstance(calibration, Mapping):
        raise ValueError("shadow scoring requires calibrated recommendation policy")
    temperature = float(calibration.get("temperature", 1.0))
    threshold = float(recommendation_policy.get("decision_threshold", 0.0))

    raw = _model_probability_one(selected_model, current.features)
    calibrated = _temperature_scale(raw, temperature)
    direction = _direction(calibrated, threshold)
    decision = DIRECTION_TO_ACTION[direction]

    profile = _profile_for_direction(recommendation_policy, direction)
    selected_features = {
        name: float(current.features[name])
        for name in selected_model.feature_names
        if name in current.features
    }
    if len(selected_features) != len(selected_model.feature_names):
        missing = [
            name
            for name in selected_model.feature_names
            if name not in current.features
        ]
        raise ValueError(
            "shadow feature vector is missing selected model features: "
            + ", ".join(missing[:8])
        )
    feature_snapshot_hash = stable_hash({
        "experiment_id": experiment_id,
        "bar_time_ms": current.bar_time_ms,
        "as_of_time_ms": current.as_of_time_ms,
        "features": selected_features,
    })
    observation_id = stable_hash({
        "experiment_id": experiment_id,
        "bar_time_ms": current.bar_time_ms,
        "model_hash": selected_model.model_hash,
        "feature_snapshot_hash": feature_snapshot_hash,
    })[:32]

    return ShadowObservation(
        observation_id=observation_id,
        experiment_id=experiment_id,
        bar_time_ms=current.bar_time_ms,
        as_of_time_ms=current.as_of_time_ms,
        created_at_ms=created_at_ms,
        origin_close=current.origin_close,
        model_algorithm=selected_model.algorithm,
        model_hash=selected_model.model_hash,
        feature_snapshot_hash=feature_snapshot_hash,
        features=selected_features,
        decision=decision,
        probabilities={
            "BUY": calibrated["UP"],
            "SELL": calibrated["DOWN"],
            "NO_TRADE": calibrated["FLAT"],
        },
        target1_pct=_profile_number(profile, "target1_pct"),
        target2_pct=_profile_number(profile, "target2_pct"),
        invalidation_pct=_profile_number(profile, "invalidation_pct"),
    )


def evaluate_shadow_observation(
    observation: ShadowObservation,
    *,
    future_bars: Sequence[Bar],
    evaluated_at_ms: int,
) -> ShadowOutcome:
    if not future_bars:
        raise ValueError("shadow outcome requires future bars")
    for bar in future_bars:
        bar.validate()
        if not bar.final or bar.quality.upper() not in USABLE_QUALITY:
            raise ValueError("shadow outcome requires finalized usable future bars")

    terminal = future_bars[-1].close
    terminal_return = (terminal / observation.origin_close) - 1.0
    high_excursions = [
        (bar.high / observation.origin_close) - 1.0
        for bar in future_bars
    ]
    low_excursions = [
        (bar.low / observation.origin_close) - 1.0
        for bar in future_bars
    ]
    mfe = max(high_excursions)
    mae = min(low_excursions)
    actual_direction = (
        "UP"
        if terminal > observation.origin_close
        else "DOWN"
        if terminal < observation.origin_close
        else "FLAT"
    )
    predicted_direction = ACTION_TO_DIRECTION[observation.decision]
    sign = 1.0 if predicted_direction == "UP" else -1.0 if predicted_direction == "DOWN" else 0.0

    probabilities = {
        "UP": float(observation.probabilities.get("BUY", 0.0)),
        "DOWN": float(observation.probabilities.get("SELL", 0.0)),
        "FLAT": float(observation.probabilities.get("NO_TRADE", 0.0)),
    }
    brier = sum(
        (probabilities[direction] - (1.0 if direction == actual_direction else 0.0)) ** 2
        for direction in ("UP", "DOWN", "FLAT")
    ) / 3.0

    t1: bool | None = None
    t2: bool | None = None
    stop: bool | None = None
    if predicted_direction in {"UP", "DOWN"} and observation.invalidation_pct is not None:
        if observation.target1_pct is not None:
            first1 = _first_hit_path(
                predicted_direction,
                high_excursions,
                low_excursions,
                observation.target1_pct,
                observation.invalidation_pct,
            )
            t1 = first1 == "TARGET"
            stop = first1 == "INVALIDATION"
        if observation.target2_pct is not None:
            first2 = _first_hit_path(
                predicted_direction,
                high_excursions,
                low_excursions,
                observation.target2_pct,
                observation.invalidation_pct,
            )
            t2 = first2 == "TARGET"

    return ShadowOutcome(
        observation_id=observation.observation_id,
        experiment_id=observation.experiment_id,
        evaluated_at_ms=evaluated_at_ms,
        actual_direction=actual_direction,
        terminal_return=terminal_return,
        mfe=mfe,
        mae=mae,
        correct=predicted_direction == actual_direction,
        strategy_return=sign * terminal_return,
        brier=brier,
        target1_before_invalidation=t1,
        target2_before_invalidation=t2,
        invalidation_before_target1=stop,
    )


def summarize_shadow(
    observations: Sequence[ShadowObservation],
    outcomes: Sequence[ShadowOutcome],
) -> ShadowSummary:
    observation_by_id = {
        item.observation_id: item
        for item in observations
    }
    outcome_by_id = {
        item.observation_id: item
        for item in outcomes
    }
    completed_pairs = [
        (observation_by_id[outcome_id], outcome)
        for outcome_id, outcome in outcome_by_id.items()
        if outcome_id in observation_by_id
    ]
    completed_pairs.sort(key=lambda pair: pair[0].as_of_time_ms)

    examples: list[LearningExample] = []
    directions: list[str] = []
    for observation, outcome in completed_pairs:
        examples.append(
            LearningExample(
                prediction_id=observation.observation_id,
                prediction_time_ms=observation.as_of_time_ms,
                feature_snapshot_hash=observation.feature_snapshot_hash,
                feature_set_version="shadow-live",
                features={},
                actual_direction=outcome.actual_direction,
                return_value=outcome.terminal_return,
                champion_direction="FLAT",
            )
        )
        directions.append(ACTION_TO_DIRECTION[observation.decision])

    metrics = evaluate_directions(examples, directions)
    brier_values = [outcome.brier for _, outcome in completed_pairs]
    target_rows = [
        outcome
        for _, outcome in completed_pairs
        if outcome.target1_before_invalidation is not None
    ]
    target2_rows = [
        outcome
        for _, outcome in completed_pairs
        if outcome.target2_before_invalidation is not None
    ]
    stop_rows = [
        outcome
        for _, outcome in completed_pairs
        if outcome.invalidation_before_target1 is not None
    ]

    return ShadowSummary(
        completed_samples=len(completed_pairs),
        pending_samples=max(0, len(observations) - len(completed_pairs)),
        metrics=metrics,
        average_brier=fmean(brier_values) if brier_values else 0.0,
        target1_samples=len(target_rows),
        target1_before_invalidation=(
            fmean(1.0 if value.target1_before_invalidation else 0.0 for value in target_rows)
            if target_rows else 0.0
        ),
        target2_before_invalidation=(
            fmean(1.0 if value.target2_before_invalidation else 0.0 for value in target2_rows)
            if target2_rows else 0.0
        ),
        invalidation_before_target1=(
            fmean(1.0 if value.invalidation_before_target1 else 0.0 for value in stop_rows)
            if stop_rows else 0.0
        ),
    )


def certification_gate(
    summary: ShadowSummary,
    *,
    backtest_metrics: Mapping[str, Any],
    config: ShadowConfig,
) -> tuple[bool, tuple[str, ...]]:
    reasons: list[str] = []
    if summary.completed_samples < config.min_samples:
        reasons.append("insufficient_shadow_samples")
    if summary.metrics.coverage < config.min_coverage:
        reasons.append("shadow_coverage_too_low")

    backtest_accuracy = float(backtest_metrics.get("accuracy", 0.0))
    if summary.metrics.accuracy + config.max_accuracy_regression < backtest_accuracy:
        reasons.append("shadow_accuracy_regression")

    backtest_average_return = float(
        backtest_metrics.get("average_strategy_return", 0.0)
    )
    if (
        summary.metrics.average_strategy_return
        + config.max_average_return_regression
        < backtest_average_return
    ):
        reasons.append("shadow_average_return_regression")

    backtest_drawdown = float(backtest_metrics.get("max_drawdown", 0.0))
    if summary.metrics.max_drawdown > backtest_drawdown + config.max_drawdown_slack:
        reasons.append("shadow_drawdown_regression")

    if summary.average_brier > config.max_brier:
        reasons.append("shadow_calibration_brier_too_high")

    if (
        summary.target1_samples > 0
        and summary.target1_before_invalidation
        < config.min_target1_before_invalidation
    ):
        reasons.append("shadow_target1_hit_rate_too_low")

    return not reasons, tuple(reasons)


def _profile_for_direction(
    recommendation_policy: Mapping[str, Any],
    direction: str,
) -> Mapping[str, Any] | None:
    profiles = recommendation_policy.get("target_profiles")
    if not isinstance(profiles, Mapping):
        return None
    value = profiles.get(direction)
    return value if isinstance(value, Mapping) else None


def _profile_number(
    profile: Mapping[str, Any] | None,
    key: str,
) -> float | None:
    if profile is None:
        return None
    value = profile.get(key)
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    number = float(value)
    return number if math.isfinite(number) and number >= 0.0 else None


def _first_hit_path(
    direction: str,
    high_excursions: Sequence[float],
    low_excursions: Sequence[float],
    target_pct: float,
    invalidation_pct: float,
) -> str:
    for high, low in zip(high_excursions, low_excursions):
        favorable = high if direction == "UP" else -low
        adverse = -low if direction == "UP" else high
        # Same-bar ambiguity remains conservative: invalidation first.
        if adverse >= invalidation_pct:
            return "INVALIDATION"
        if favorable >= target_pct:
            return "TARGET"
    return "NONE"


def _optional_float(value: Any) -> float | None:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError("shadow optional numeric value is invalid")
    number = float(value)
    if not math.isfinite(number):
        raise ValueError("shadow optional numeric value must be finite")
    return number


def _optional_bool(value: Any) -> bool | None:
    if value is None:
        return None
    if not isinstance(value, bool):
        raise ValueError("shadow optional boolean value is invalid")
    return value
