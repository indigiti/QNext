from __future__ import annotations

from dataclasses import dataclass
import math
from statistics import fmean, median
from typing import Any, Mapping, Sequence

from .domain import stable_hash
from .lab_ml import LabMLCandidate
from .lab_snapshot import HistoricalLabExample, LabCurrentFeatureVector
from .learning import (
    CandidateModel,
    DIRECTIONS,
    EvaluationMetrics,
    LearnedLinearModel,
    LearningDataset,
    LearningExample,
    evaluate_directions,
)

SCHEMA = "QNEXT.INTELLIGENCE.LAB.RECOMMENDATION/1"
ACTION_MAP = {"UP": "BUY", "DOWN": "SELL", "FLAT": "NO_TRADE"}
EPSILON = 1e-9


@dataclass(frozen=True, slots=True)
class CalibrationResult:
    method: str
    temperature: float
    validation_log_loss_before: float
    validation_log_loss_after: float
    test_log_loss: float
    test_brier: float

    def to_record(self) -> dict[str, Any]:
        return {
            "method": self.method,
            "temperature": self.temperature,
            "validation_log_loss_before": self.validation_log_loss_before,
            "validation_log_loss_after": self.validation_log_loss_after,
            "test_log_loss": self.test_log_loss,
            "test_brier": self.test_brier,
        }


@dataclass(frozen=True, slots=True)
class TargetProfile:
    direction: str
    samples: int
    target1_pct: float
    target2_pct: float
    invalidation_pct: float
    expected_side_return: float
    expected_horizon_bars: float
    validation_target1_before_invalidation: float
    validation_target2_before_invalidation: float
    test_samples: int
    test_target1_before_invalidation: float
    test_target2_before_invalidation: float
    test_invalidation_before_target1: float

    def to_record(self) -> dict[str, Any]:
        return {
            "direction": self.direction,
            "samples": self.samples,
            "target1_pct": self.target1_pct,
            "target2_pct": self.target2_pct,
            "invalidation_pct": self.invalidation_pct,
            "expected_side_return": self.expected_side_return,
            "expected_horizon_bars": self.expected_horizon_bars,
            "validation_target1_before_invalidation": self.validation_target1_before_invalidation,
            "validation_target2_before_invalidation": self.validation_target2_before_invalidation,
            "test_samples": self.test_samples,
            "test_target1_before_invalidation": self.test_target1_before_invalidation,
            "test_target2_before_invalidation": self.test_target2_before_invalidation,
            "test_invalidation_before_target1": self.test_invalidation_before_target1,
        }


@dataclass(frozen=True, slots=True)
class LabRecommendation:
    recommendation_id: str
    experiment_id: str
    created_at_ms: int
    as_of_time_ms: int
    model_algorithm: str
    model_hash: str
    feature_set_version: str
    probability_calibrated: bool
    calibration: CalibrationResult
    decision_threshold: float
    test_metrics: EvaluationMetrics
    entry_price: float
    decision: str
    probabilities: Mapping[str, float]
    target_profile: TargetProfile | None
    target1_price: float | None
    target2_price: float | None
    invalidation_price: float | None
    expected_return_pct: float | None
    expected_horizon_bars: float | None
    target_profiles: Mapping[str, TargetProfile]

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": SCHEMA,
            "recommendation_id": self.recommendation_id,
            "experiment_id": self.experiment_id,
            "created_at_ms": self.created_at_ms,
            "as_of_time_ms": self.as_of_time_ms,
            "model_algorithm": self.model_algorithm,
            "model_hash": self.model_hash,
            "feature_set_version": self.feature_set_version,
            "probability_calibrated": self.probability_calibrated,
            "calibration": self.calibration.to_record(),
            "decision_threshold": self.decision_threshold,
            "test_metrics": self.test_metrics.to_record(),
            "entry_price": self.entry_price,
            "decision": self.decision,
            "probabilities": dict(self.probabilities),
            "target_profile": self.target_profile.to_record() if self.target_profile else None,
            "target1_price": self.target1_price,
            "target2_price": self.target2_price,
            "invalidation_price": self.invalidation_price,
            "expected_return_pct": self.expected_return_pct,
            "expected_horizon_bars": self.expected_horizon_bars,
            "target_profiles": {
                key: value.to_record()
                for key, value in sorted(self.target_profiles.items())
            },
        }


def _chronological_split(dataset: LearningDataset):
    examples = dataset.examples
    train_end = max(1, int(len(examples) * 0.60))
    validation_end = max(train_end + 1, int(len(examples) * 0.80))
    validation_end = min(validation_end, len(examples) - 1)
    train = examples[:train_end]
    validation = examples[train_end:validation_end]
    test = examples[validation_end:]
    if not train or not validation or not test:
        raise ValueError("recommendation calibration needs train, validation and test windows")
    return train, validation, test


def _softmax(values: Sequence[float]) -> list[float]:
    if not values:
        return []
    maximum = max(values)
    weights = [math.exp(value - maximum) for value in values]
    total = sum(weights)
    if total <= 0:
        return [1.0 / len(values) for _ in values]
    return [value / total for value in weights]


def _normalize_probability_map(value: Mapping[str, float]) -> dict[str, float]:
    raw = {direction: max(float(value.get(direction, 0.0)), 0.0) for direction in DIRECTIONS}
    total = sum(raw.values())
    if total <= 0:
        return {direction: 1.0 / len(DIRECTIONS) for direction in DIRECTIONS}
    return {direction: raw[direction] / total for direction in DIRECTIONS}


def _temperature_scale(probabilities: Mapping[str, float], temperature: float) -> dict[str, float]:
    if temperature <= 0:
        raise ValueError("calibration temperature must be positive")
    normalized = _normalize_probability_map(probabilities)
    logits = [math.log(max(normalized[direction], EPSILON)) / temperature for direction in DIRECTIONS]
    scaled = _softmax(logits)
    return {direction: scaled[index] for index, direction in enumerate(DIRECTIONS)}


def _log_loss(rows: Sequence[Mapping[str, float]], actual: Sequence[str]) -> float:
    if not rows or len(rows) != len(actual):
        raise ValueError("probability rows and actual labels must align")
    return -fmean(
        math.log(max(_normalize_probability_map(row)[label], EPSILON))
        for row, label in zip(rows, actual)
    )


def _brier(rows: Sequence[Mapping[str, float]], actual: Sequence[str]) -> float:
    if not rows or len(rows) != len(actual):
        raise ValueError("probability rows and actual labels must align")
    values: list[float] = []
    for row, label in zip(rows, actual):
        normalized = _normalize_probability_map(row)
        values.append(
            sum(
                (normalized[direction] - (1.0 if direction == label else 0.0)) ** 2
                for direction in DIRECTIONS
            ) / len(DIRECTIONS)
        )
    return fmean(values)


def _fit_temperature(
    rows: Sequence[Mapping[str, float]],
    actual: Sequence[str],
) -> tuple[float, float, float]:
    before = _log_loss(rows, actual)
    best_temperature = 1.0
    best_loss = before
    for temperature in (0.50, 0.67, 0.80, 1.0, 1.25, 1.50, 2.0, 3.0):
        scaled = [_temperature_scale(row, temperature) for row in rows]
        loss = _log_loss(scaled, actual)
        if (loss, abs(temperature - 1.0)) < (best_loss, abs(best_temperature - 1.0)):
            best_temperature = temperature
            best_loss = loss
    return best_temperature, before, best_loss


def _sklearn_payload_probability(
    payload: Mapping[str, Any],
    features: Mapping[str, float],
    feature_names: Sequence[str],
) -> dict[str, float]:
    classes = [str(value).upper() for value in payload.get("classes", [])]
    means = [float(value) for value in payload.get("scaler_mean", [])]
    scales = [float(value) for value in payload.get("scaler_scale", [])]
    coefficients = payload.get("coefficients", [])
    intercepts = [float(value) for value in payload.get("intercepts", [])]
    if len(means) != len(feature_names) or len(scales) != len(feature_names):
        raise ValueError("scikit-learn recommendation payload feature shape mismatch")
    row = [
        (float(features[name]) - means[index]) / max(scales[index], EPSILON)
        for index, name in enumerate(feature_names)
    ]

    logits: list[float] = []
    for index, raw_coefficients in enumerate(coefficients):
        values = [float(value) for value in raw_coefficients]
        intercept = intercepts[index] if index < len(intercepts) else 0.0
        logits.append(intercept + sum(value * row[pos] for pos, value in enumerate(values)))

    if len(classes) == 2 and len(logits) == 1:
        positive = 1.0 / (1.0 + math.exp(-max(min(logits[0], 50.0), -50.0)))
        result = {classes[0]: 1.0 - positive, classes[1]: positive}
    elif len(classes) == len(logits) and logits:
        probabilities = _softmax(logits)
        result = {classes[index]: probabilities[index] for index in range(len(classes))}
    else:
        raise ValueError("scikit-learn recommendation payload class shape mismatch")
    return _normalize_probability_map(result)


def _lightgbm_payload_probabilities(
    payload: Mapping[str, Any],
    rows: Sequence[Mapping[str, float]],
    feature_names: Sequence[str],
) -> list[dict[str, float]]:
    try:
        import lightgbm
    except Exception as exc:
        raise RuntimeError("LightGBM is required to score the selected Lab model") from exc

    model_string = str(payload.get("model_string", ""))
    classes = [str(value).upper() for value in payload.get("classes", [])]
    if not model_string or not classes:
        raise ValueError("LightGBM recommendation payload is incomplete")
    booster = lightgbm.Booster(model_str=model_string)
    matrix = [[float(row[name]) for name in feature_names] for row in rows]
    predicted = booster.predict(matrix)

    output: list[dict[str, float]] = []
    if len(classes) == 2:
        for raw in predicted:
            positive = float(raw)
            output.append(_normalize_probability_map({
                classes[0]: 1.0 - positive,
                classes[1]: positive,
            }))
        return output

    for raw in predicted:
        values = [float(value) for value in raw]
        output.append(_normalize_probability_map({
            classes[index]: values[index]
            for index in range(min(len(classes), len(values)))
        }))
    return output


def _model_probabilities(
    model: CandidateModel | LabMLCandidate,
    examples: Sequence[LearningExample],
) -> list[dict[str, float]]:
    if isinstance(model, CandidateModel):
        learned = LearnedLinearModel(model)
        return [
            _normalize_probability_map(learned.probabilities(example.features))
            for example in examples
        ]

    payload = model.model_payload
    payload_type = str(payload.get("type", ""))
    if payload_type == "sklearn-logistic-v1":
        return [
            _sklearn_payload_probability(payload, example.features, model.feature_names)
            for example in examples
        ]
    if payload_type == "lightgbm-booster-v1":
        return _lightgbm_payload_probabilities(
            payload,
            [example.features for example in examples],
            model.feature_names,
        )
    raise ValueError("unsupported Lab recommendation model payload")


def _model_probability_one(
    model: CandidateModel | LabMLCandidate,
    features: Mapping[str, float],
) -> dict[str, float]:
    missing = [name for name in model.feature_names if name not in features]
    if missing:
        raise ValueError(
            "latest Lab feature vector is missing selected model features: "
            + ", ".join(missing[:8])
        )

    if isinstance(model, CandidateModel):
        learned = LearnedLinearModel(model)
        return _normalize_probability_map(learned.probabilities(features))

    payload = model.model_payload
    payload_type = str(payload.get("type", ""))
    if payload_type == "sklearn-logistic-v1":
        return _sklearn_payload_probability(payload, features, model.feature_names)
    if payload_type == "lightgbm-booster-v1":
        return _lightgbm_payload_probabilities(payload, [features], model.feature_names)[0]
    raise ValueError("unsupported Lab recommendation model payload")


def _direction(
    probabilities: Mapping[str, float],
    threshold: float,
) -> str:
    normalized = _normalize_probability_map(probabilities)
    directional = "UP" if normalized["UP"] >= normalized["DOWN"] else "DOWN"
    confidence = normalized[directional]
    if normalized["FLAT"] >= confidence or confidence < threshold:
        return "FLAT"
    return directional


def _objective(metrics: EvaluationMetrics) -> tuple[float, float, float]:
    return (
        metrics.average_strategy_return - (0.25 * metrics.max_drawdown),
        metrics.accuracy,
        metrics.coverage,
    )


def _fit_decision_threshold(
    examples: Sequence[LearningExample],
    calibrated: Sequence[Mapping[str, float]],
) -> tuple[float, EvaluationMetrics]:
    best: tuple[tuple[float, float, float], float, EvaluationMetrics] | None = None
    for threshold in (0.45, 0.50, 0.55, 0.60, 0.65, 0.70, 0.75):
        directions = [_direction(row, threshold) for row in calibrated]
        metrics = evaluate_directions(examples, directions)
        choice = (_objective(metrics), -threshold, metrics)
        if best is None or choice[:2] > best[:2]:
            best = choice
    if best is None:
        raise ValueError("no recommendation decision threshold was evaluated")
    return -best[1], best[2]


def _quantile(values: Sequence[float], fraction: float) -> float:
    if not values:
        raise ValueError("cannot compute recommendation quantile from no values")
    ordered = sorted(float(value) for value in values)
    if len(ordered) == 1:
        return ordered[0]
    position = (len(ordered) - 1) * fraction
    lower = int(math.floor(position))
    upper = int(math.ceil(position))
    if lower == upper:
        return ordered[lower]
    weight = position - lower
    return ordered[lower] * (1.0 - weight) + ordered[upper] * weight


def _side_metrics(example: HistoricalLabExample, direction: str) -> tuple[float, float, float]:
    if direction == "UP":
        return max(example.mfe, 0.0), max(-example.mae, 0.0), example.return_value
    if direction == "DOWN":
        return max(-example.mae, 0.0), max(example.mfe, 0.0), -example.return_value
    raise ValueError("target profile direction must be UP or DOWN")


def _first_hit(
    example: HistoricalLabExample,
    direction: str,
    target_pct: float,
    invalidation_pct: float,
) -> tuple[str, int | None]:
    for index, (high, low) in enumerate(
        zip(example.future_high_excursions, example.future_low_excursions),
        start=1,
    ):
        favorable = high if direction == "UP" else -low
        adverse = -low if direction == "UP" else high
        # Conservative ambiguity policy: if both can occur inside one OHLC bar,
        # invalidation is counted first because intrabar ordering is unknown.
        if adverse >= invalidation_pct:
            return "INVALIDATION", index
        if favorable >= target_pct:
            return "TARGET", index
    return "NONE", None


def _profile_hit_rates(
    examples: Sequence[HistoricalLabExample],
    direction: str,
    target1: float,
    target2: float,
    invalidation: float,
) -> tuple[float, float, float, list[int]]:
    if not examples:
        return 0.0, 0.0, 0.0, []

    t1 = 0
    t2 = 0
    stop = 0
    target1_bars: list[int] = []
    for example in examples:
        first1, bar1 = _first_hit(example, direction, target1, invalidation)
        first2, _ = _first_hit(example, direction, target2, invalidation)
        if first1 == "TARGET":
            t1 += 1
            if bar1 is not None:
                target1_bars.append(bar1)
        elif first1 == "INVALIDATION":
            stop += 1
        if first2 == "TARGET":
            t2 += 1

    total = len(examples)
    return t1 / total, t2 / total, stop / total, target1_bars


def _build_target_profile(
    direction: str,
    validation_examples: Sequence[HistoricalLabExample],
    test_examples: Sequence[HistoricalLabExample],
) -> TargetProfile | None:
    if len(validation_examples) < 5:
        return None

    favorable: list[float] = []
    adverse: list[float] = []
    side_returns: list[float] = []
    for example in validation_examples:
        move, risk, side_return = _side_metrics(example, direction)
        favorable.append(move)
        adverse.append(risk)
        side_returns.append(side_return)

    target1 = max(_quantile(favorable, 0.50), EPSILON)
    target2 = max(_quantile(favorable, 0.75), target1)
    invalidation = max(_quantile(adverse, 0.75), EPSILON)

    validation_t1, validation_t2, _, target1_bars = _profile_hit_rates(
        validation_examples,
        direction,
        target1,
        target2,
        invalidation,
    )
    test_t1, test_t2, test_stop, _ = _profile_hit_rates(
        test_examples,
        direction,
        target1,
        target2,
        invalidation,
    )
    expected_horizon = float(median(target1_bars)) if target1_bars else float(
        median(
            [
                example.mfe_bar if direction == "UP" else example.mae_bar
                for example in validation_examples
            ]
        )
    )

    return TargetProfile(
        direction=direction,
        samples=len(validation_examples),
        target1_pct=target1,
        target2_pct=target2,
        invalidation_pct=invalidation,
        expected_side_return=float(median(side_returns)),
        expected_horizon_bars=expected_horizon,
        validation_target1_before_invalidation=validation_t1,
        validation_target2_before_invalidation=validation_t2,
        test_samples=len(test_examples),
        test_target1_before_invalidation=test_t1,
        test_target2_before_invalidation=test_t2,
        test_invalidation_before_target1=test_stop,
    )


def _price_levels(
    entry: float,
    direction: str,
    profile: TargetProfile | None,
) -> tuple[float | None, float | None, float | None]:
    if profile is None:
        return None, None, None
    if direction == "UP":
        return (
            entry * (1.0 + profile.target1_pct),
            entry * (1.0 + profile.target2_pct),
            entry * (1.0 - profile.invalidation_pct),
        )
    if direction == "DOWN":
        return (
            entry * (1.0 - profile.target1_pct),
            entry * (1.0 - profile.target2_pct),
            entry * (1.0 + profile.invalidation_pct),
        )
    return None, None, None


def build_recommendation(
    *,
    experiment_id: str,
    dataset: LearningDataset,
    historical_examples: Sequence[HistoricalLabExample],
    current: LabCurrentFeatureVector,
    selected_model: CandidateModel | LabMLCandidate,
    created_at_ms: int,
) -> LabRecommendation:
    _, validation, test = _chronological_split(dataset)
    validation_raw = _model_probabilities(selected_model, validation)
    test_raw = _model_probabilities(selected_model, test)

    temperature, before_loss, after_loss = _fit_temperature(
        validation_raw,
        [example.actual_direction for example in validation],
    )
    validation_calibrated = [
        _temperature_scale(row, temperature)
        for row in validation_raw
    ]
    test_calibrated = [
        _temperature_scale(row, temperature)
        for row in test_raw
    ]
    threshold, _ = _fit_decision_threshold(validation, validation_calibrated)
    test_directions = [_direction(row, threshold) for row in test_calibrated]
    test_metrics = evaluate_directions(test, test_directions)

    historical_by_time = {
        example.as_of_time_ms: example
        for example in historical_examples
    }
    validation_by_side: dict[str, list[HistoricalLabExample]] = {"UP": [], "DOWN": []}
    test_by_side: dict[str, list[HistoricalLabExample]] = {"UP": [], "DOWN": []}

    for example, row in zip(validation, validation_calibrated):
        direction = _direction(row, threshold)
        historical = historical_by_time.get(example.prediction_time_ms)
        if direction in validation_by_side and historical is not None:
            validation_by_side[direction].append(historical)

    for example, row in zip(test, test_calibrated):
        direction = _direction(row, threshold)
        historical = historical_by_time.get(example.prediction_time_ms)
        if direction in test_by_side and historical is not None:
            test_by_side[direction].append(historical)

    profiles: dict[str, TargetProfile] = {}
    for direction in ("UP", "DOWN"):
        profile = _build_target_profile(
            direction,
            validation_by_side[direction],
            test_by_side[direction],
        )
        if profile is not None:
            profiles[direction] = profile

    latest_raw = _model_probability_one(selected_model, current.features)
    latest = _temperature_scale(latest_raw, temperature)
    latest_direction = _direction(latest, threshold)
    decision = ACTION_MAP[latest_direction]
    profile = profiles.get(latest_direction)
    target1, target2, invalidation = _price_levels(
        current.origin_close,
        latest_direction,
        profile,
    )

    calibration = CalibrationResult(
        method="temperature-scaling-v1",
        temperature=temperature,
        validation_log_loss_before=before_loss,
        validation_log_loss_after=after_loss,
        test_log_loss=_log_loss(
            test_calibrated,
            [example.actual_direction for example in test],
        ),
        test_brier=_brier(
            test_calibrated,
            [example.actual_direction for example in test],
        ),
    )
    model_algorithm = selected_model.algorithm
    model_hash = selected_model.model_hash
    material = {
        "experiment_id": experiment_id,
        "as_of_time_ms": current.as_of_time_ms,
        "model_algorithm": model_algorithm,
        "model_hash": model_hash,
        "feature_set_version": dataset.feature_set_version,
        "temperature": temperature,
        "decision_threshold": threshold,
        "decision": decision,
        "probabilities": latest,
        "entry_price": current.origin_close,
        "profiles": {key: value.to_record() for key, value in sorted(profiles.items())},
    }
    recommendation_id = stable_hash(material)[:32]

    return LabRecommendation(
        recommendation_id=recommendation_id,
        experiment_id=experiment_id,
        created_at_ms=created_at_ms,
        as_of_time_ms=current.as_of_time_ms,
        model_algorithm=model_algorithm,
        model_hash=model_hash,
        feature_set_version=dataset.feature_set_version,
        probability_calibrated=True,
        calibration=calibration,
        decision_threshold=threshold,
        test_metrics=test_metrics,
        entry_price=current.origin_close,
        decision=decision,
        probabilities={
            "BUY": latest["UP"],
            "SELL": latest["DOWN"],
            "NO_TRADE": latest["FLAT"],
        },
        target_profile=profile,
        target1_price=target1,
        target2_price=target2,
        invalidation_price=invalidation,
        expected_return_pct=profile.expected_side_return if profile else None,
        expected_horizon_bars=profile.expected_horizon_bars if profile else None,
        target_profiles=profiles,
    )
