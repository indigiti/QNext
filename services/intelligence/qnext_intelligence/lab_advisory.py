from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Mapping

from .domain import stable_hash
from .lab import LabExperiment
from .lab_ml import LabMLCandidate
from .lab_shadow import ACTION_TO_DIRECTION, score_shadow_observation
from .lab_snapshot import LabCurrentFeatureVector
from .learning import CandidateModel

SCHEMA = "QNEXT.INTELLIGENCE.CERTIFIED_ADVISORY/1"


@dataclass(frozen=True, slots=True)
class CertifiedAdvisory:
    advisory_id: str
    experiment_id: str
    instrument_id: str
    timeframe: str
    indicator_configuration_hash: str
    feature_schema_version: str
    certified_at_ms: int
    bar_time_ms: int
    as_of_time_ms: int
    created_at_ms: int
    model_algorithm: str
    model_hash: str
    decision: str
    probabilities: Mapping[str, float]
    entry_price: float
    target1_price: float | None
    target2_price: float | None
    invalidation_price: float | None
    expected_return_pct: float | None
    expected_horizon_bars: float | None
    recommendation_policy_id: str

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": SCHEMA,
            "advisory_id": self.advisory_id,
            "experiment_id": self.experiment_id,
            "instrument_id": self.instrument_id,
            "timeframe": self.timeframe,
            "indicator_configuration_hash": self.indicator_configuration_hash,
            "feature_schema_version": self.feature_schema_version,
            "certified_at_ms": self.certified_at_ms,
            "bar_time_ms": self.bar_time_ms,
            "as_of_time_ms": self.as_of_time_ms,
            "created_at_ms": self.created_at_ms,
            "model_algorithm": self.model_algorithm,
            "model_hash": self.model_hash,
            "decision": self.decision,
            "probabilities": dict(self.probabilities),
            "entry_price": self.entry_price,
            "target1_price": self.target1_price,
            "target2_price": self.target2_price,
            "invalidation_price": self.invalidation_price,
            "expected_return_pct": self.expected_return_pct,
            "expected_horizon_bars": self.expected_horizon_bars,
            "recommendation_policy_id": self.recommendation_policy_id,
        }


def build_certified_advisory(
    *,
    experiment: LabExperiment,
    current: LabCurrentFeatureVector,
    selected_model: CandidateModel | LabMLCandidate,
    recommendation_policy: Mapping[str, Any],
    certified_at_ms: int,
    created_at_ms: int,
) -> CertifiedAdvisory:
    if experiment.lifecycle_state != "CERTIFIED":
        raise ValueError("certified advisory requires CERTIFIED Lab state")
    if not bool(recommendation_policy.get("probability_calibrated", False)):
        raise ValueError("certified advisory requires calibrated recommendation policy")

    observation = score_shadow_observation(
        experiment_id=experiment.experiment_id,
        current=current,
        selected_model=selected_model,
        recommendation_policy=recommendation_policy,
        created_at_ms=created_at_ms,
    )

    direction = ACTION_TO_DIRECTION[observation.decision]
    profile = _target_profile(recommendation_policy, direction)
    target1_price, target2_price, invalidation_price = _price_levels(
        current.origin_close,
        direction,
        observation.target1_pct,
        observation.target2_pct,
        observation.invalidation_pct,
    )
    policy_id = str(recommendation_policy.get("recommendation_id", ""))
    if not policy_id:
        raise ValueError("certified advisory recommendation policy id is missing")

    material = {
        "experiment_id": experiment.experiment_id,
        "instrument_id": experiment.instrument_id,
        "timeframe": experiment.timeframe,
        "indicator_configuration_hash": experiment.indicator_configuration_hash,
        "feature_schema_version": experiment.feature_schema_version,
        "certified_at_ms": certified_at_ms,
        "bar_time_ms": current.bar_time_ms,
        "as_of_time_ms": current.as_of_time_ms,
        "model_algorithm": selected_model.algorithm,
        "model_hash": selected_model.model_hash,
        "decision": observation.decision,
        "probabilities": dict(observation.probabilities),
        "entry_price": current.origin_close,
        "target1_price": target1_price,
        "target2_price": target2_price,
        "invalidation_price": invalidation_price,
        "expected_return_pct": _profile_float(profile, "expected_side_return"),
        "expected_horizon_bars": _profile_float(profile, "expected_horizon_bars"),
        "recommendation_policy_id": policy_id,
    }
    advisory_id = stable_hash(material)[:32]

    return CertifiedAdvisory(
        advisory_id=advisory_id,
        experiment_id=experiment.experiment_id,
        instrument_id=experiment.instrument_id,
        timeframe=experiment.timeframe,
        indicator_configuration_hash=experiment.indicator_configuration_hash,
        feature_schema_version=experiment.feature_schema_version,
        certified_at_ms=certified_at_ms,
        bar_time_ms=current.bar_time_ms,
        as_of_time_ms=current.as_of_time_ms,
        created_at_ms=created_at_ms,
        model_algorithm=selected_model.algorithm,
        model_hash=selected_model.model_hash,
        decision=observation.decision,
        probabilities=dict(observation.probabilities),
        entry_price=current.origin_close,
        target1_price=target1_price,
        target2_price=target2_price,
        invalidation_price=invalidation_price,
        expected_return_pct=_profile_float(profile, "expected_side_return"),
        expected_horizon_bars=_profile_float(profile, "expected_horizon_bars"),
        recommendation_policy_id=policy_id,
    )


def _target_profile(
    recommendation_policy: Mapping[str, Any],
    direction: str,
) -> Mapping[str, Any] | None:
    profiles = recommendation_policy.get("target_profiles")
    if not isinstance(profiles, Mapping):
        return None
    value = profiles.get(direction)
    return value if isinstance(value, Mapping) else None


def _profile_float(
    profile: Mapping[str, Any] | None,
    key: str,
) -> float | None:
    if profile is None:
        return None
    value = profile.get(key)
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return float(value)


def _price_levels(
    entry: float,
    direction: str,
    target1_pct: float | None,
    target2_pct: float | None,
    invalidation_pct: float | None,
) -> tuple[float | None, float | None, float | None]:
    if direction == "UP":
        return (
            None if target1_pct is None else entry * (1.0 + target1_pct),
            None if target2_pct is None else entry * (1.0 + target2_pct),
            None if invalidation_pct is None else entry * (1.0 - invalidation_pct),
        )
    if direction == "DOWN":
        return (
            None if target1_pct is None else entry * (1.0 - target1_pct),
            None if target2_pct is None else entry * (1.0 - target2_pct),
            None if invalidation_pct is None else entry * (1.0 + invalidation_pct),
        )
    return None, None, None
