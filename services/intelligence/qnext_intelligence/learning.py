from __future__ import annotations

from dataclasses import asdict, dataclass
import math
from statistics import fmean, pstdev
from typing import Any, Mapping, Sequence

from .domain import ModelManifest, stable_hash

USABLE_QUALITY = {"GOOD", "RECOVERED"}
DIRECTIONS = {"UP", "DOWN", "FLAT"}
ALGORITHM = "qnext-ridge-direction-v1"


@dataclass(frozen=True, slots=True)
class LearningExample:
    prediction_id: str
    prediction_time_ms: int
    feature_snapshot_hash: str
    feature_set_version: str
    features: Mapping[str, float]
    actual_direction: str
    return_value: float
    champion_direction: str


@dataclass(frozen=True, slots=True)
class LearningDataset:
    examples: tuple[LearningExample, ...]
    feature_names: tuple[str, ...]
    feature_set_version: str
    dataset_hash: str

    @property
    def start_ms(self) -> int:
        return self.examples[0].prediction_time_ms if self.examples else 0

    @property
    def end_ms(self) -> int:
        return self.examples[-1].prediction_time_ms if self.examples else 0


@dataclass(frozen=True, slots=True)
class EvaluationMetrics:
    samples: int
    accuracy: float
    coverage: float
    strategy_return: float
    average_strategy_return: float
    max_drawdown: float
    trades: int

    def to_record(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True, slots=True)
class PromotionGate:
    passed: bool
    reasons: tuple[str, ...]

    def to_record(self) -> dict[str, Any]:
        return {"passed": self.passed, "reasons": list(self.reasons)}


@dataclass(frozen=True, slots=True)
class CandidateModel:
    candidate_id: str
    model_name: str
    model_version: str
    lifecycle_state: str
    algorithm: str
    feature_set_version: str
    dataset_hash: str
    model_hash: str
    feature_names: tuple[str, ...]
    means: tuple[float, ...]
    scales: tuple[float, ...]
    coefficients: tuple[float, ...]
    intercept: float
    flat_threshold: float
    ridge: float
    training_window_start_ms: int
    training_window_end_ms: int
    validation_window_start_ms: int
    validation_window_end_ms: int
    test_window_start_ms: int
    test_window_end_ms: int
    created_at_ms: int
    train_samples: int
    validation_samples: int
    test_samples: int
    validation_metrics: EvaluationMetrics
    test_metrics: EvaluationMetrics
    champion_test_metrics: EvaluationMetrics
    promotion_gate: PromotionGate

    def to_record(self) -> dict[str, Any]:
        record = asdict(self)
        record["promotion_gate"] = self.promotion_gate.to_record()
        record["validation_metrics"] = self.validation_metrics.to_record()
        record["test_metrics"] = self.test_metrics.to_record()
        record["champion_test_metrics"] = self.champion_test_metrics.to_record()
        return record

    @classmethod
    def from_record(cls, record: Mapping[str, Any]) -> "CandidateModel":
        return cls(
            candidate_id=str(record["candidate_id"]),
            model_name=str(record["model_name"]),
            model_version=str(record["model_version"]),
            lifecycle_state=str(record["lifecycle_state"]),
            algorithm=str(record["algorithm"]),
            feature_set_version=str(record["feature_set_version"]),
            dataset_hash=str(record["dataset_hash"]),
            model_hash=str(record["model_hash"]),
            feature_names=tuple(str(value) for value in record["feature_names"]),
            means=tuple(float(value) for value in record["means"]),
            scales=tuple(float(value) for value in record["scales"]),
            coefficients=tuple(float(value) for value in record["coefficients"]),
            intercept=float(record["intercept"]),
            flat_threshold=float(record["flat_threshold"]),
            ridge=float(record["ridge"]),
            training_window_start_ms=int(record["training_window_start_ms"]),
            training_window_end_ms=int(record["training_window_end_ms"]),
            validation_window_start_ms=int(record["validation_window_start_ms"]),
            validation_window_end_ms=int(record["validation_window_end_ms"]),
            test_window_start_ms=int(record["test_window_start_ms"]),
            test_window_end_ms=int(record["test_window_end_ms"]),
            created_at_ms=int(record["created_at_ms"]),
            train_samples=int(record["train_samples"]),
            validation_samples=int(record["validation_samples"]),
            test_samples=int(record["test_samples"]),
            validation_metrics=EvaluationMetrics(**record["validation_metrics"]),
            test_metrics=EvaluationMetrics(**record["test_metrics"]),
            champion_test_metrics=EvaluationMetrics(**record["champion_test_metrics"]),
            promotion_gate=PromotionGate(
                passed=bool(record["promotion_gate"]["passed"]),
                reasons=tuple(str(value) for value in record["promotion_gate"].get("reasons", [])),
            ),
        )

    def manifest(self, lifecycle_state: str | None = None, promoted_at_ms: int = 0) -> ModelManifest:
        state = lifecycle_state or self.lifecycle_state
        return ModelManifest(
            model_name=self.model_name,
            model_version=self.model_version,
            lifecycle_state=state,
            feature_set_version=self.feature_set_version,
            dataset_hash=self.dataset_hash,
            model_hash=self.model_hash,
            algorithm=self.algorithm,
            training_window_start_ms=self.training_window_start_ms,
            training_window_end_ms=self.training_window_end_ms,
            created_at_ms=self.created_at_ms,
            promoted_at_ms=promoted_at_ms,
        )


class LearnedLinearModel:
    def __init__(self, candidate: CandidateModel) -> None:
        self.candidate = candidate

    def score(self, values: Mapping[str, float]) -> float:
        total = self.candidate.intercept
        for index, name in enumerate(self.candidate.feature_names):
            value = float(values.get(name, 0.0))
            normalized = (value - self.candidate.means[index]) / self.candidate.scales[index]
            total += self.candidate.coefficients[index] * normalized
        return total

    def direction(self, values: Mapping[str, float]) -> str:
        score = self.score(values)
        if abs(score) <= self.candidate.flat_threshold:
            return "FLAT"
        return "UP" if score > 0.0 else "DOWN"

    def probabilities(self, values: Mapping[str, float]) -> Mapping[str, float]:
        score = self.score(values)
        magnitude = abs(score)
        confidence = math.tanh(magnitude)
        if magnitude <= self.candidate.flat_threshold:
            flat = 0.55 + (0.35 * (1.0 - confidence))
            residual = 1.0 - flat
            return {"UP": residual / 2.0, "DOWN": residual / 2.0, "FLAT": flat}
        directional = 0.55 + (0.4 * confidence)
        flat = 1.0 - directional
        if score > 0.0:
            return {"UP": directional, "DOWN": 0.0, "FLAT": flat}
        return {"UP": 0.0, "DOWN": directional, "FLAT": flat}


def build_learning_dataset(
    feature_records: Sequence[Mapping[str, Any]],
    prediction_records: Sequence[Mapping[str, Any]],
    outcome_records: Sequence[Mapping[str, Any]],
) -> LearningDataset:
    features_by_hash = {
        str(record["snapshot_hash"]): record
        for record in feature_records
        if record.get("snapshot_hash")
    }
    outcomes_by_prediction = {
        str(record["prediction_id"]): record
        for record in outcome_records
        if record.get("prediction_id")
    }

    examples: list[LearningExample] = []
    seen: set[str] = set()
    feature_names: tuple[str, ...] | None = None
    feature_set_version = ""

    for prediction in prediction_records:
        prediction_id = str(prediction.get("prediction_id", ""))
        if not prediction_id or prediction_id in seen:
            continue
        seen.add(prediction_id)
        if str(prediction.get("state", "")).upper() != "IMMUTABLE":
            continue
        snapshot_hash = str(prediction.get("feature_snapshot_hash", ""))
        feature = features_by_hash.get(snapshot_hash)
        outcome = outcomes_by_prediction.get(prediction_id)
        if feature is None or outcome is None:
            continue
        quality = str(feature.get("data_quality", "")).upper()
        if quality not in USABLE_QUALITY:
            continue
        actual = str(outcome.get("direction_actual", "")).upper()
        champion = str(prediction.get("direction", "")).upper()
        if actual not in DIRECTIONS or champion not in DIRECTIONS:
            continue

        values = feature.get("features")
        if not isinstance(values, Mapping) or not values:
            continue
        names = tuple(sorted(str(name) for name in values))
        if feature_names is None:
            feature_names = names
        elif names != feature_names:
            raise ValueError("learning dataset contains inconsistent feature columns")

        version = str(feature.get("feature_set_version", ""))
        if not feature_set_version:
            feature_set_version = version
        elif version != feature_set_version:
            raise ValueError("learning dataset mixes feature set versions")

        examples.append(
            LearningExample(
                prediction_id=prediction_id,
                prediction_time_ms=int(prediction["prediction_time_ms"]),
                feature_snapshot_hash=snapshot_hash,
                feature_set_version=version,
                features={name: float(values[name]) for name in names},
                actual_direction=actual,
                return_value=float(outcome.get("return_value", 0.0)),
                champion_direction=champion,
            )
        )

    examples.sort(key=lambda item: (item.prediction_time_ms, item.prediction_id))
    if not examples or feature_names is None:
        raise ValueError("no complete learning examples are available")

    material = [
        {
            "prediction_id": item.prediction_id,
            "prediction_time_ms": item.prediction_time_ms,
            "feature_snapshot_hash": item.feature_snapshot_hash,
            "feature_set_version": item.feature_set_version,
            "features": dict(item.features),
            "actual_direction": item.actual_direction,
            "return_value": item.return_value,
            "champion_direction": item.champion_direction,
        }
        for item in examples
    ]
    return LearningDataset(
        examples=tuple(examples),
        feature_names=feature_names,
        feature_set_version=feature_set_version,
        dataset_hash=stable_hash(material),
    )


def _direction_target(direction: str) -> float:
    if direction == "UP":
        return 1.0
    if direction == "DOWN":
        return -1.0
    return 0.0


def _fit_standardizer(
    examples: Sequence[LearningExample], feature_names: Sequence[str]
) -> tuple[tuple[float, ...], tuple[float, ...]]:
    columns = [[float(item.features[name]) for item in examples] for name in feature_names]
    means = tuple(fmean(column) for column in columns)
    scales = tuple(max(pstdev(column), 1e-12) for column in columns)
    return means, scales


def _design_row(
    example: LearningExample,
    feature_names: Sequence[str],
    means: Sequence[float],
    scales: Sequence[float],
) -> list[float]:
    return [1.0] + [
        (float(example.features[name]) - means[index]) / scales[index]
        for index, name in enumerate(feature_names)
    ]


def _solve_linear_system(matrix: list[list[float]], vector: list[float]) -> list[float]:
    size = len(vector)
    augmented = [matrix[row][:] + [vector[row]] for row in range(size)]
    for column in range(size):
        pivot = max(range(column, size), key=lambda row: abs(augmented[row][column]))
        if abs(augmented[pivot][column]) < 1e-12:
            raise ValueError("singular regression system")
        if pivot != column:
            augmented[column], augmented[pivot] = augmented[pivot], augmented[column]
        divisor = augmented[column][column]
        augmented[column] = [value / divisor for value in augmented[column]]
        for row in range(size):
            if row == column:
                continue
            factor = augmented[row][column]
            if factor == 0.0:
                continue
            augmented[row] = [
                augmented[row][index] - factor * augmented[column][index]
                for index in range(size + 1)
            ]
    return [augmented[row][-1] for row in range(size)]


def _fit_ridge(
    examples: Sequence[LearningExample],
    feature_names: Sequence[str],
    means: Sequence[float],
    scales: Sequence[float],
    ridge: float,
) -> tuple[float, tuple[float, ...]]:
    width = len(feature_names) + 1
    xtx = [[0.0 for _ in range(width)] for _ in range(width)]
    xty = [0.0 for _ in range(width)]
    for example in examples:
        row = _design_row(example, feature_names, means, scales)
        target = _direction_target(example.actual_direction)
        for left in range(width):
            xty[left] += row[left] * target
            for right in range(width):
                xtx[left][right] += row[left] * row[right]
    for index in range(1, width):
        xtx[index][index] += ridge
    weights = _solve_linear_system(xtx, xty)
    return weights[0], tuple(weights[1:])


def _score(
    example: LearningExample,
    feature_names: Sequence[str],
    means: Sequence[float],
    scales: Sequence[float],
    intercept: float,
    coefficients: Sequence[float],
) -> float:
    total = intercept
    for index, name in enumerate(feature_names):
        total += coefficients[index] * (
            (float(example.features[name]) - means[index]) / scales[index]
        )
    return total


def _direction_from_score(score: float, flat_threshold: float) -> str:
    if abs(score) <= flat_threshold:
        return "FLAT"
    return "UP" if score > 0.0 else "DOWN"


def evaluate_directions(
    examples: Sequence[LearningExample], directions: Sequence[str]
) -> EvaluationMetrics:
    if len(examples) != len(directions):
        raise ValueError("example and direction lengths differ")
    if not examples:
        return EvaluationMetrics(0, 0.0, 0.0, 0.0, 0.0, 0.0, 0)

    correct = 0
    trades = 0
    cumulative = 0.0
    peak = 0.0
    max_drawdown = 0.0
    for example, direction in zip(examples, directions):
        if direction not in DIRECTIONS:
            raise ValueError("invalid evaluation direction")
        if direction == example.actual_direction:
            correct += 1
        sign = 1.0 if direction == "UP" else -1.0 if direction == "DOWN" else 0.0
        if sign != 0.0:
            trades += 1
        cumulative += sign * example.return_value
        peak = max(peak, cumulative)
        max_drawdown = max(max_drawdown, peak - cumulative)

    samples = len(examples)
    return EvaluationMetrics(
        samples=samples,
        accuracy=correct / samples,
        coverage=trades / samples,
        strategy_return=cumulative,
        average_strategy_return=cumulative / samples,
        max_drawdown=max_drawdown,
        trades=trades,
    )


def _evaluate_model(
    examples: Sequence[LearningExample],
    feature_names: Sequence[str],
    means: Sequence[float],
    scales: Sequence[float],
    intercept: float,
    coefficients: Sequence[float],
    flat_threshold: float,
) -> EvaluationMetrics:
    directions = [
        _direction_from_score(
            _score(example, feature_names, means, scales, intercept, coefficients),
            flat_threshold,
        )
        for example in examples
    ]
    return evaluate_directions(examples, directions)


def _objective(metrics: EvaluationMetrics) -> tuple[float, float, float]:
    return (
        metrics.average_strategy_return - (0.25 * metrics.max_drawdown),
        metrics.accuracy,
        metrics.coverage,
    )


def train_candidate(
    dataset: LearningDataset,
    *,
    model_name: str = "qnext-self-learning-direction",
    created_at_ms: int,
    min_samples: int = 60,
    min_test_samples: int = 12,
    min_average_return_improvement: float = 0.0,
    max_accuracy_regression: float = 0.02,
    max_drawdown_slack: float = 0.01,
    ridge_values: Sequence[float] = (0.001, 0.01, 0.1, 1.0, 10.0),
    flat_thresholds: Sequence[float] = (0.0, 0.10, 0.20, 0.30),
) -> CandidateModel:
    examples = dataset.examples
    if len(examples) < min_samples:
        raise ValueError(f"need at least {min_samples} complete learning examples")
    if created_at_ms <= dataset.end_ms:
        raise ValueError("candidate creation time must be after the learning dataset")

    train_end = max(1, int(len(examples) * 0.60))
    validation_end = max(train_end + 1, int(len(examples) * 0.80))
    validation_end = min(validation_end, len(examples) - 1)
    train = examples[:train_end]
    validation = examples[train_end:validation_end]
    test = examples[validation_end:]
    if len(test) < min_test_samples:
        raise ValueError("untouched test window is smaller than required minimum")

    means, scales = _fit_standardizer(train, dataset.feature_names)

    best = None
    for ridge in ridge_values:
        if ridge <= 0.0:
            raise ValueError("ridge values must be positive")
        intercept, coefficients = _fit_ridge(
            train, dataset.feature_names, means, scales, float(ridge)
        )
        for threshold in flat_thresholds:
            if threshold < 0.0:
                raise ValueError("flat thresholds cannot be negative")
            metrics = _evaluate_model(
                validation,
                dataset.feature_names,
                means,
                scales,
                intercept,
                coefficients,
                float(threshold),
            )
            choice = (
                _objective(metrics),
                -float(ridge),
                -float(threshold),
                coefficients,
                intercept,
                metrics,
            )
            if best is None or choice[:3] > best[:3]:
                best = choice

    if best is None:
        raise ValueError("no candidate hyperparameters were evaluated")
    _, negative_ridge, negative_threshold, coefficients, intercept, validation_metrics = best
    ridge = -negative_ridge
    flat_threshold = -negative_threshold

    test_metrics = _evaluate_model(
        test,
        dataset.feature_names,
        means,
        scales,
        intercept,
        coefficients,
        flat_threshold,
    )
    champion_metrics = evaluate_directions(
        test, [example.champion_direction for example in test]
    )

    reasons: list[str] = []
    if test_metrics.samples < min_test_samples:
        reasons.append("insufficient_test_samples")
    if (
        test_metrics.average_strategy_return
        < champion_metrics.average_strategy_return + min_average_return_improvement
    ):
        reasons.append("average_return_not_improved")
    if test_metrics.accuracy + max_accuracy_regression < champion_metrics.accuracy:
        reasons.append("accuracy_regression")
    if test_metrics.max_drawdown > champion_metrics.max_drawdown + max_drawdown_slack:
        reasons.append("drawdown_regression")
    gate = PromotionGate(passed=not reasons, reasons=tuple(reasons))

    model_material = {
        "algorithm": ALGORITHM,
        "feature_set_version": dataset.feature_set_version,
        "dataset_hash": dataset.dataset_hash,
        "feature_names": list(dataset.feature_names),
        "means": list(means),
        "scales": list(scales),
        "coefficients": list(coefficients),
        "intercept": intercept,
        "flat_threshold": flat_threshold,
        "ridge": ridge,
        "training_window": [train[0].prediction_time_ms, train[-1].prediction_time_ms],
        "validation_window": [validation[0].prediction_time_ms, validation[-1].prediction_time_ms],
        "test_window": [test[0].prediction_time_ms, test[-1].prediction_time_ms],
    }
    model_hash = stable_hash(model_material)
    candidate_id = stable_hash(
        {
            "model_name": model_name,
            "model_hash": model_hash,
            "dataset_hash": dataset.dataset_hash,
        }
    )[:32]

    return CandidateModel(
        candidate_id=candidate_id,
        model_name=model_name,
        model_version=candidate_id[:12],
        lifecycle_state="DRAFT",
        algorithm=ALGORITHM,
        feature_set_version=dataset.feature_set_version,
        dataset_hash=dataset.dataset_hash,
        model_hash=model_hash,
        feature_names=dataset.feature_names,
        means=means,
        scales=scales,
        coefficients=coefficients,
        intercept=intercept,
        flat_threshold=flat_threshold,
        ridge=ridge,
        training_window_start_ms=train[0].prediction_time_ms,
        training_window_end_ms=train[-1].prediction_time_ms,
        validation_window_start_ms=validation[0].prediction_time_ms,
        validation_window_end_ms=validation[-1].prediction_time_ms,
        test_window_start_ms=test[0].prediction_time_ms,
        test_window_end_ms=test[-1].prediction_time_ms,
        created_at_ms=created_at_ms,
        train_samples=len(train),
        validation_samples=len(validation),
        test_samples=len(test),
        validation_metrics=validation_metrics,
        test_metrics=test_metrics,
        champion_test_metrics=champion_metrics,
        promotion_gate=gate,
    )
