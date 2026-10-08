from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any, Mapping, Sequence

from .domain import stable_hash
from .learning import EvaluationMetrics, LearningDataset, PromotionGate, evaluate_directions

DIRECTIONS = ("UP", "DOWN", "FLAT")
SCHEMA = "QNEXT.INTELLIGENCE.LAB.ML_CANDIDATE/1"


@dataclass(frozen=True, slots=True)
class FamilyValidation:
    family: str
    algorithm: str
    params: Mapping[str, Any]
    confidence_threshold: float
    validation_metrics: EvaluationMetrics
    model_payload: Mapping[str, Any]

    def to_record(self) -> dict[str, Any]:
        return {
            "family": self.family,
            "algorithm": self.algorithm,
            "params": dict(self.params),
            "confidence_threshold": self.confidence_threshold,
            "validation_metrics": self.validation_metrics.to_record(),
        }


@dataclass(frozen=True, slots=True)
class LabMLCandidate:
    candidate_id: str
    lifecycle_state: str
    algorithm: str
    family: str
    feature_set_version: str
    dataset_hash: str
    model_hash: str
    feature_names: tuple[str, ...]
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
    params: Mapping[str, Any]
    confidence_threshold: float
    validation_metrics: EvaluationMetrics
    test_metrics: EvaluationMetrics
    ridge_validation_metrics: EvaluationMetrics
    ridge_test_metrics: EvaluationMetrics
    promotion_gate: PromotionGate
    library_versions: Mapping[str, str]
    probability_calibrated: bool
    model_payload: Mapping[str, Any]
    family_validation: tuple[Mapping[str, Any], ...]

    def to_record(self) -> dict[str, Any]:
        return {
            "schema": SCHEMA,
            "candidate_id": self.candidate_id,
            "lifecycle_state": self.lifecycle_state,
            "algorithm": self.algorithm,
            "family": self.family,
            "feature_set_version": self.feature_set_version,
            "dataset_hash": self.dataset_hash,
            "model_hash": self.model_hash,
            "feature_names": list(self.feature_names),
            "training_window_start_ms": self.training_window_start_ms,
            "training_window_end_ms": self.training_window_end_ms,
            "validation_window_start_ms": self.validation_window_start_ms,
            "validation_window_end_ms": self.validation_window_end_ms,
            "test_window_start_ms": self.test_window_start_ms,
            "test_window_end_ms": self.test_window_end_ms,
            "created_at_ms": self.created_at_ms,
            "train_samples": self.train_samples,
            "validation_samples": self.validation_samples,
            "test_samples": self.test_samples,
            "params": dict(self.params),
            "confidence_threshold": self.confidence_threshold,
            "validation_metrics": self.validation_metrics.to_record(),
            "test_metrics": self.test_metrics.to_record(),
            "ridge_validation_metrics": self.ridge_validation_metrics.to_record(),
            "ridge_test_metrics": self.ridge_test_metrics.to_record(),
            "promotion_gate": self.promotion_gate.to_record(),
            "library_versions": dict(self.library_versions),
            "probability_calibrated": self.probability_calibrated,
            "model_payload": dict(self.model_payload),
            "family_validation": list(self.family_validation),
        }


def dependency_status() -> dict[str, Any]:
    status: dict[str, Any] = {
        "scikit_learn": {"available": False, "version": ""},
        "lightgbm": {"available": False, "version": ""},
    }
    try:
        import sklearn

        status["scikit_learn"] = {
            "available": True,
            "version": str(sklearn.__version__),
        }
    except Exception:
        pass
    try:
        import lightgbm

        status["lightgbm"] = {
            "available": True,
            "version": str(lightgbm.__version__),
        }
    except Exception:
        pass
    return status


def _objective(metrics: EvaluationMetrics) -> tuple[float, float, float]:
    return (
        metrics.average_strategy_return - (0.25 * metrics.max_drawdown),
        metrics.accuracy,
        metrics.coverage,
    )


def _split(dataset: LearningDataset, min_test_samples: int):
    examples = dataset.examples
    train_end = max(1, int(len(examples) * 0.60))
    validation_end = max(train_end + 1, int(len(examples) * 0.80))
    validation_end = min(validation_end, len(examples) - 1)
    train = examples[:train_end]
    validation = examples[train_end:validation_end]
    test = examples[validation_end:]
    if not train or not validation or len(test) < min_test_samples:
        raise ValueError("ML tournament does not have enough chronological samples")
    return train, validation, test


def _xy(examples, feature_names: Sequence[str]):
    x = [[float(item.features[name]) for name in feature_names] for item in examples]
    y = [item.actual_direction for item in examples]
    return x, y


def _directions_from_probabilities(
    probabilities: Sequence[Sequence[float]],
    classes: Sequence[str],
    confidence_threshold: float,
) -> list[str]:
    directions: list[str] = []
    normalized_classes = [str(value).upper() for value in classes]
    for row in probabilities:
        if not row:
            directions.append("FLAT")
            continue
        best_index = max(range(len(row)), key=lambda index: float(row[index]))
        confidence = float(row[best_index])
        direction = normalized_classes[best_index]
        if confidence < confidence_threshold:
            direction = "FLAT"
        if direction not in DIRECTIONS:
            raise ValueError("ML model emitted unsupported direction")
        directions.append(direction)
    return directions


def _best_threshold(examples, probabilities, classes) -> tuple[float, EvaluationMetrics]:
    best: tuple[tuple[float, float, float], float, EvaluationMetrics] | None = None
    for threshold in (0.0, 0.45, 0.50, 0.55, 0.60, 0.65):
        directions = _directions_from_probabilities(
            probabilities,
            classes,
            threshold,
        )
        metrics = evaluate_directions(examples, directions)
        choice = (_objective(metrics), -threshold, metrics)
        if best is None or choice[:2] > best[:2]:
            best = choice
    if best is None:
        raise ValueError("no ML confidence threshold was evaluated")
    return -best[1], best[2]


def _fit_sklearn_logistic(dataset: LearningDataset, train, validation) -> FamilyValidation:
    try:
        import sklearn
        from sklearn.linear_model import LogisticRegression
        from sklearn.preprocessing import StandardScaler
    except Exception as exc:
        raise RuntimeError("scikit-learn is not installed") from exc

    x_train, y_train = _xy(train, dataset.feature_names)
    x_validation, _ = _xy(validation, dataset.feature_names)
    if len(set(y_train)) < 2:
        raise ValueError("scikit-learn challenger needs at least two training classes")

    best = None
    for c_value in (0.1, 1.0, 10.0):
        for class_weight in (None, "balanced"):
            scaler = StandardScaler()
            train_scaled = scaler.fit_transform(x_train)
            validation_scaled = scaler.transform(x_validation)
            model = LogisticRegression(
                C=c_value,
                class_weight=class_weight,
                max_iter=1000,
                random_state=42,
                solver="lbfgs",
            )
            model.fit(train_scaled, y_train)
            probabilities = model.predict_proba(validation_scaled).tolist()
            threshold, metrics = _best_threshold(
                validation,
                probabilities,
                model.classes_.tolist(),
            )
            params = {
                "C": c_value,
                "class_weight": class_weight or "none",
                "solver": "lbfgs",
            }
            payload = {
                "type": "sklearn-logistic-v1",
                "classes": [str(value) for value in model.classes_.tolist()],
                "scaler_mean": [float(value) for value in scaler.mean_.tolist()],
                "scaler_scale": [float(value) for value in scaler.scale_.tolist()],
                "coefficients": [
                    [float(value) for value in row]
                    for row in model.coef_.tolist()
                ],
                "intercepts": [float(value) for value in model.intercept_.tolist()],
            }
            choice = (
                _objective(metrics),
                -c_value,
                0 if class_weight is None else -1,
                threshold,
                params,
                payload,
                metrics,
            )
            if best is None or choice[:4] > best[:4]:
                best = choice

    if best is None:
        raise ValueError("no scikit-learn challenger was trained")
    _, _, _, threshold, params, payload, metrics = best
    return FamilyValidation(
        family="scikit-learn",
        algorithm="sklearn-logistic-direction-v1",
        params=params,
        confidence_threshold=threshold,
        validation_metrics=metrics,
        model_payload={
            **payload,
            "library_version": str(sklearn.__version__),
        },
    )


def _fit_lightgbm(dataset: LearningDataset, train, validation) -> FamilyValidation:
    try:
        import lightgbm
        from lightgbm import LGBMClassifier
    except Exception as exc:
        raise RuntimeError("LightGBM is not installed") from exc

    x_train, y_train = _xy(train, dataset.feature_names)
    x_validation, _ = _xy(validation, dataset.feature_names)
    if len(set(y_train)) < 2:
        raise ValueError("LightGBM challenger needs at least two training classes")

    best = None
    for learning_rate, num_leaves, n_estimators in (
        (0.05, 15, 120),
        (0.05, 31, 180),
        (0.10, 15, 120),
        (0.10, 31, 180),
    ):
        model = LGBMClassifier(
            objective="multiclass" if len(set(y_train)) > 2 else "binary",
            learning_rate=learning_rate,
            num_leaves=num_leaves,
            n_estimators=n_estimators,
            reg_lambda=0.1,
            subsample=1.0,
            colsample_bytree=1.0,
            random_state=42,
            n_jobs=1,
            deterministic=True,
            force_col_wise=True,
            verbosity=-1,
        )
        model.fit(x_train, y_train)
        probabilities = model.predict_proba(x_validation).tolist()
        threshold, metrics = _best_threshold(
            validation,
            probabilities,
            model.classes_.tolist(),
        )
        params = {
            "learning_rate": learning_rate,
            "num_leaves": num_leaves,
            "n_estimators": n_estimators,
            "reg_lambda": 0.1,
        }
        booster = model.booster_
        payload = {
            "type": "lightgbm-booster-v1",
            "classes": [str(value) for value in model.classes_.tolist()],
            "model_string": booster.model_to_string(),
            "library_version": str(lightgbm.__version__),
        }
        choice = (
            _objective(metrics),
            -learning_rate,
            -num_leaves,
            -n_estimators,
            threshold,
            params,
            payload,
            metrics,
        )
        if best is None or choice[:5] > best[:5]:
            best = choice

    if best is None:
        raise ValueError("no LightGBM challenger was trained")
    _, _, _, _, threshold, params, payload, metrics = best
    return FamilyValidation(
        family="lightgbm",
        algorithm="lightgbm-direction-v1",
        params=params,
        confidence_threshold=threshold,
        validation_metrics=metrics,
        model_payload=payload,
    )


def _refit_selected(
    selected: FamilyValidation,
    dataset: LearningDataset,
    train,
):
    x_train, y_train = _xy(train, dataset.feature_names)

    if selected.family == "scikit-learn":
        import sklearn
        from sklearn.linear_model import LogisticRegression
        from sklearn.preprocessing import StandardScaler

        class_weight = selected.params.get("class_weight")
        scaler = StandardScaler()
        train_scaled = scaler.fit_transform(x_train)
        model = LogisticRegression(
            C=float(selected.params["C"]),
            class_weight=None if class_weight == "none" else class_weight,
            max_iter=1000,
            random_state=42,
            solver="lbfgs",
        )
        model.fit(train_scaled, y_train)

        def probabilities(x):
            return model.predict_proba(scaler.transform(x)).tolist()

        payload = {
            "type": "sklearn-logistic-v1",
            "classes": [str(value) for value in model.classes_.tolist()],
            "scaler_mean": [float(value) for value in scaler.mean_.tolist()],
            "scaler_scale": [float(value) for value in scaler.scale_.tolist()],
            "coefficients": [
                [float(value) for value in row]
                for row in model.coef_.tolist()
            ],
            "intercepts": [float(value) for value in model.intercept_.tolist()],
            "library_version": str(sklearn.__version__),
        }
        return model.classes_.tolist(), probabilities, payload

    import lightgbm
    from lightgbm import LGBMClassifier

    model = LGBMClassifier(
        objective="multiclass" if len(set(y_train)) > 2 else "binary",
        learning_rate=float(selected.params["learning_rate"]),
        num_leaves=int(selected.params["num_leaves"]),
        n_estimators=int(selected.params["n_estimators"]),
        reg_lambda=float(selected.params["reg_lambda"]),
        subsample=1.0,
        colsample_bytree=1.0,
        random_state=42,
        n_jobs=1,
        deterministic=True,
        force_col_wise=True,
        verbosity=-1,
    )
    model.fit(x_train, y_train)

    def probabilities(x):
        return model.predict_proba(x).tolist()

    payload = {
        "type": "lightgbm-booster-v1",
        "classes": [str(value) for value in model.classes_.tolist()],
        "model_string": model.booster_.model_to_string(),
        "library_version": str(lightgbm.__version__),
    }
    return model.classes_.tolist(), probabilities, payload


def train_ml_challenger(
    dataset: LearningDataset,
    *,
    ridge_validation_metrics: EvaluationMetrics,
    ridge_test_metrics: EvaluationMetrics,
    created_at_ms: int,
    min_test_samples: int = 12,
    min_average_return_improvement: float = 0.0,
    max_accuracy_regression: float = 0.02,
    max_drawdown_slack: float = 0.01,
) -> LabMLCandidate | None:
    train, validation, test = _split(dataset, min_test_samples)

    families: list[FamilyValidation] = []
    for trainer in (_fit_sklearn_logistic, _fit_lightgbm):
        try:
            families.append(trainer(dataset, train, validation))
        except (RuntimeError, ValueError):
            continue

    if not families:
        return None

    selected = max(
        families,
        key=lambda item: (_objective(item.validation_metrics), item.algorithm),
    )
    if _objective(selected.validation_metrics) <= _objective(ridge_validation_metrics):
        return None

    classes, probability_fn, model_payload = _refit_selected(selected, dataset, train)
    x_test, _ = _xy(test, dataset.feature_names)
    directions = _directions_from_probabilities(
        probability_fn(x_test),
        classes,
        selected.confidence_threshold,
    )
    test_metrics = evaluate_directions(test, directions)

    reasons: list[str] = []
    if test_metrics.samples < min_test_samples:
        reasons.append("insufficient_test_samples")
    if (
        test_metrics.average_strategy_return
        < ridge_test_metrics.average_strategy_return + min_average_return_improvement
    ):
        reasons.append("average_return_not_improved_vs_ridge")
    if test_metrics.accuracy + max_accuracy_regression < ridge_test_metrics.accuracy:
        reasons.append("accuracy_regression_vs_ridge")
    if test_metrics.max_drawdown > ridge_test_metrics.max_drawdown + max_drawdown_slack:
        reasons.append("drawdown_regression_vs_ridge")
    gate = PromotionGate(passed=not reasons, reasons=tuple(reasons))

    status = dependency_status()
    versions = {
        "scikit_learn": str(status["scikit_learn"]["version"]),
        "lightgbm": str(status["lightgbm"]["version"]),
    }
    material = {
        "algorithm": selected.algorithm,
        "feature_set_version": dataset.feature_set_version,
        "dataset_hash": dataset.dataset_hash,
        "feature_names": list(dataset.feature_names),
        "params": dict(selected.params),
        "confidence_threshold": selected.confidence_threshold,
        "model_payload": model_payload,
        "training_window": [train[0].prediction_time_ms, train[-1].prediction_time_ms],
        "validation_window": [
            validation[0].prediction_time_ms,
            validation[-1].prediction_time_ms,
        ],
        "test_window": [test[0].prediction_time_ms, test[-1].prediction_time_ms],
    }
    model_hash = stable_hash(material)
    candidate_id = stable_hash({
        "algorithm": selected.algorithm,
        "model_hash": model_hash,
        "dataset_hash": dataset.dataset_hash,
    })[:32]

    return LabMLCandidate(
        candidate_id=candidate_id,
        lifecycle_state="DRAFT",
        algorithm=selected.algorithm,
        family=selected.family,
        feature_set_version=dataset.feature_set_version,
        dataset_hash=dataset.dataset_hash,
        model_hash=model_hash,
        feature_names=dataset.feature_names,
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
        params=selected.params,
        confidence_threshold=selected.confidence_threshold,
        validation_metrics=selected.validation_metrics,
        test_metrics=test_metrics,
        ridge_validation_metrics=ridge_validation_metrics,
        ridge_test_metrics=ridge_test_metrics,
        promotion_gate=gate,
        library_versions=versions,
        probability_calibrated=False,
        model_payload=model_payload,
        family_validation=tuple(item.to_record() for item in families),
    )
