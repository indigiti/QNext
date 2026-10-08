from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
from statistics import pstdev
import time
from typing import Any, Mapping, Sequence

from .client import MarketCoreHistoryClient, timeframe_to_milliseconds
from .domain import stable_hash
from .lab import IntelligenceLabRegistry, LabEvaluation
from .lab_snapshot import (
    HistoricalLabExample,
    build_historical_examples,
    build_latest_feature_vector,
    historical_dataset_hash,
    import_chart_snapshot,
)
from .learning import CandidateModel, build_learning_dataset, train_candidate
from .lab_ml import LabMLCandidate, SCHEMA as LAB_ML_SCHEMA, dependency_status, train_ml_challenger
from .lab_recommendation import build_recommendation
from .lab_shadow import (
    ShadowConfig,
    ShadowObservation,
    ShadowOutcome,
    certification_gate,
    evaluate_shadow_observation,
    score_shadow_observation,
    summarize_shadow,
)

REQUEST_SCHEMA = "QNEXT.INTELLIGENCE.LAB.REQUEST/1"


def _write_json(path: Path, value: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_suffix(path.suffix + ".tmp")
    payload = json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n"
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o640)
    try:
        os.write(fd, payload.encode("utf-8"))
        os.fsync(fd)
    finally:
        os.close(fd)
    os.replace(temp, path)


def _import_snapshot(
    registry: IntelligenceLabRegistry,
    payload: Mapping[str, Any],
) -> dict[str, Any]:
    raw_snapshot = payload.get("snapshot")
    if not isinstance(raw_snapshot, Mapping):
        raise ValueError("snapshot is required")

    imported = import_chart_snapshot(raw_snapshot)
    requested_name = payload.get("name")
    name = str(requested_name).strip() if isinstance(requested_name, str) else ""
    experiment = imported.experiment(name=name or None)
    registry.create(experiment)
    registry.save_chart_snapshot(
        experiment.experiment_id,
        raw_snapshot,
        snapshot_hash=imported.snapshot_hash,
    )
    return {
        "experiment": experiment.to_record(),
        "snapshot_hash": imported.snapshot_hash,
        "feature_rows": len(imported.feature_rows),
        "indicator_count": len(imported.indicator_ids),
    }


def _selected_feature_names(
    examples: Sequence[HistoricalLabExample],
    *,
    indicator_ids: Sequence[str] = (),
    min_coverage: float = 0.80,
    max_features: int = 96,
) -> tuple[str, ...]:
    if not examples:
        return ()

    counts: dict[str, int] = {}
    values: dict[str, list[float]] = {}
    for example in examples:
        for name, value in example.features.items():
            counts[name] = counts.get(name, 0) + 1
            values.setdefault(name, []).append(float(value))

    eligible: list[str] = []
    minimum = max(1, int(len(examples) * min_coverage))
    for name, count in counts.items():
        column = values.get(name, [])
        if count < minimum or len(column) < 2:
            continue
        if pstdev(column) <= 1e-12:
            continue
        eligible.append(name)

    def priority(name: str) -> tuple[int, str]:
        if name.startswith("market."):
            return (0, name)
        if name.endswith(".delta_1"):
            return (2, name)
        if name.endswith(".delta_3"):
            return (3, name)
        if name.endswith(".z20"):
            return (4, name)
        return (1, name)

    ordered = sorted(eligible, key=priority)
    mandatory: list[str] = []
    for indicator_id in indicator_ids:
        prefix = f"indicator.{_safe_feature_part(indicator_id)}."
        candidates = [name for name in ordered if name.startswith(prefix)]
        if candidates:
            mandatory.append(candidates[0])

    mandatory = list(dict.fromkeys(mandatory))
    if len(mandatory) > max_features:
        raise ValueError("max feature limit cannot represent every enabled indicator")

    selected = mandatory[:]
    for name in ordered:
        if name in selected:
            continue
        if len(selected) >= max_features:
            break
        selected.append(name)

    if not selected:
        raise ValueError("historical examples have no stable non-constant features")
    return tuple(selected)


def _safe_feature_part(value: str) -> str:
    normalized = re.sub(r"[^a-z0-9]+", "_", value.strip().lower()).strip("_")
    return normalized or "unnamed"


def _indicator_feature_coverage(
    indicator_ids: Sequence[str],
    feature_names: Sequence[str],
) -> tuple[dict[str, int], tuple[str, ...]]:
    counts: dict[str, int] = {}
    missing: list[str] = []
    for indicator_id in indicator_ids:
        prefix = f"indicator.{_safe_feature_part(indicator_id)}."
        count = sum(1 for name in feature_names if name.startswith(prefix))
        counts[indicator_id] = count
        if count == 0:
            missing.append(indicator_id)
    return counts, tuple(missing)


def _learning_records(
    experiment_id: str,
    feature_schema_version: str,
    examples: Sequence[HistoricalLabExample],
    *,
    indicator_ids: Sequence[str] = (),
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[dict[str, Any]]]:
    names = _selected_feature_names(examples, indicator_ids=indicator_ids)

    features: list[dict[str, Any]] = []
    predictions: list[dict[str, Any]] = []
    outcomes: list[dict[str, Any]] = []

    accepted_index = 0
    for example in examples:
        if any(name not in example.features for name in names):
            continue
        values = {name: float(example.features[name]) for name in names}
        index = accepted_index
        accepted_index += 1
        snapshot_hash = stable_hash({
            "experiment_id": experiment_id,
            "bar_time_ms": example.bar_time_ms,
            "as_of_time_ms": example.as_of_time_ms,
            "features": values,
        })
        prediction_id = stable_hash({
            "experiment_id": experiment_id,
            "snapshot_hash": snapshot_hash,
            "index": index,
        })[:32]
        features.append({
            "snapshot_hash": snapshot_hash,
            "feature_set_version": feature_schema_version,
            "instrument_id": "",
            "timeframe": "",
            "as_of_time_ms": example.as_of_time_ms,
            "features": values,
            "data_quality": "GOOD",
        })
        predictions.append({
            "prediction_id": prediction_id,
            "prediction_time_ms": example.as_of_time_ms,
            "feature_snapshot_hash": snapshot_hash,
            "state": "IMMUTABLE",
            "direction": "FLAT",
        })
        outcomes.append({
            "prediction_id": prediction_id,
            "direction_actual": example.direction_actual,
            "return_value": example.return_value,
            "mfe": example.mfe,
            "mae": example.mae,
        })
    return features, predictions, outcomes


def _backtest(
    registry: IntelligenceLabRegistry,
    market_core_url: str,
    payload: Mapping[str, Any],
) -> dict[str, Any]:
    experiment_id = str(payload.get("experimentId", ""))
    if not experiment_id:
        raise ValueError("experimentId is required")
    experiment = registry.get(experiment_id)
    stored = registry.load_chart_snapshot(experiment_id)
    raw_snapshot = stored["payload"]
    imported = import_chart_snapshot(raw_snapshot)

    if imported.instrument_id != experiment.instrument_id or imported.timeframe != experiment.timeframe:
        raise ValueError("stored snapshot market does not match experiment")
    if imported.indicator_configuration_hash != experiment.indicator_configuration_hash:
        raise ValueError("stored snapshot configuration does not match experiment")
    if not imported.feature_rows:
        missing = ", ".join(imported.indicator_ids)
        raise ValueError(
            "experiment has no historical indicator features; "
            f"feature contracts are required for: {missing}"
        )

    horizon_bars = int(payload.get("horizonBars", 3))
    if horizon_bars < 1 or horizon_bars > 100:
        raise ValueError("horizonBars must be between 1 and 100")
    min_samples = int(payload.get("minSamples", 60))
    min_test_samples = int(payload.get("minTestSamples", 12))
    if min_samples < 30 or min_samples > 100000:
        raise ValueError("minSamples is out of range")
    if min_test_samples < 6 or min_test_samples > 50000:
        raise ValueError("minTestSamples is out of range")

    first_time = imported.feature_rows[0].bar_time_ms
    last_time = imported.feature_rows[-1].bar_time_ms
    duration = timeframe_to_milliseconds(imported.timeframe)
    client = MarketCoreHistoryClient(market_core_url)
    bars = client.fetch_bars(
        instrument_id=imported.instrument_id,
        timeframe=imported.timeframe,
        from_ms=max(0, first_time),
        to_ms=last_time + duration * (horizon_bars + 2),
    )
    examples = build_historical_examples(
        imported,
        bars,
        horizon_bars=horizon_bars,
    )
    dataset_hash = historical_dataset_hash(imported, examples)

    feature_records, prediction_records, outcome_records = _learning_records(
        experiment_id,
        imported.feature_schema_version,
        examples,
        indicator_ids=imported.indicator_ids,
    )
    dataset = build_learning_dataset(
        feature_records,
        prediction_records,
        outcome_records,
    )
    indicator_feature_counts, missing_indicators = _indicator_feature_coverage(
        imported.indicator_ids,
        dataset.feature_names,
    )

    created_at_ms = max(int(time.time() * 1000), dataset.end_ms + 1)
    candidate = train_candidate(
        dataset,
        model_name="qnext-lab-ridge-direction",
        created_at_ms=created_at_ms,
        min_samples=min_samples,
        min_test_samples=min_test_samples,
        min_average_return_improvement=float(payload.get("minAverageReturnImprovement", 0.0)),
        max_accuracy_regression=float(payload.get("maxAccuracyRegression", 0.02)),
        max_drawdown_slack=float(payload.get("maxDrawdownSlack", 0.01)),
    )
    candidate_record = candidate.to_record()
    registry.save_candidate_artifact(experiment_id, candidate_record)

    ml_candidate = None
    ml_error = ""
    try:
        ml_candidate = train_ml_challenger(
            dataset,
            ridge_validation_metrics=candidate.validation_metrics,
            ridge_test_metrics=candidate.test_metrics,
            created_at_ms=created_at_ms,
            min_test_samples=min_test_samples,
            min_average_return_improvement=float(payload.get("minAverageReturnImprovement", 0.0)),
            max_accuracy_regression=float(payload.get("maxAccuracyRegression", 0.02)),
            max_drawdown_slack=float(payload.get("maxDrawdownSlack", 0.01)),
        )
    except Exception as error:
        # Lab ML is optional and must never make the deterministic ridge
        # benchmark or production intelligence unavailable.
        ml_error = str(error)[:500]
    ml_record = ml_candidate.to_record() if ml_candidate is not None else None
    if ml_record is not None:
        registry.save_candidate_artifact(experiment_id, ml_record)

    selected_model = candidate
    selected_algorithm = candidate.algorithm
    selected_hash = candidate.model_hash
    selected_metrics = candidate.test_metrics
    selected_gate = candidate.promotion_gate
    if ml_candidate is not None and ml_candidate.promotion_gate.passed:
        selected_model = ml_candidate
        selected_algorithm = ml_candidate.algorithm
        selected_hash = ml_candidate.model_hash
        selected_metrics = ml_candidate.test_metrics
        selected_gate = ml_candidate.promotion_gate

    metrics = {
        "samples": float(selected_metrics.samples),
        "accuracy": selected_metrics.accuracy,
        "coverage": selected_metrics.coverage,
        "strategy_return": selected_metrics.strategy_return,
        "average_strategy_return": selected_metrics.average_strategy_return,
        "max_drawdown": selected_metrics.max_drawdown,
        "trades": float(selected_metrics.trades),
        "ridge_accuracy": candidate.test_metrics.accuracy,
        "ridge_average_strategy_return": candidate.test_metrics.average_strategy_return,
        "ridge_max_drawdown": candidate.test_metrics.max_drawdown,
        "champion_accuracy": candidate.champion_test_metrics.accuracy,
        "champion_average_strategy_return": candidate.champion_test_metrics.average_strategy_return,
        "champion_max_drawdown": candidate.champion_test_metrics.max_drawdown,
        "historical_examples": float(len(examples)),
        "feature_count": float(len(dataset.feature_names)),
        "enabled_indicator_count": float(len(imported.indicator_ids)),
        "covered_indicator_count": float(len(imported.indicator_ids) - len(missing_indicators)),
        "indicator_coverage_ratio": (
            (len(imported.indicator_ids) - len(missing_indicators))
            / len(imported.indicator_ids)
            if imported.indicator_ids
            else 0.0
        ),
    }
    evaluation_reasons = list(selected_gate.reasons)
    evaluation_reasons.extend(
        f"missing_historical_indicator_features:{indicator_id}"
        for indicator_id in missing_indicators
    )
    evaluation = LabEvaluation(
        experiment_id=experiment_id,
        phase="BACKTEST",
        evaluated_at_ms=created_at_ms,
        dataset_hash=dataset_hash,
        model_spec_hash=selected_hash,
        gate_passed=selected_gate.passed and not missing_indicators,
        metrics=metrics,
        reasons=tuple(evaluation_reasons),
    )
    updated = registry.record_backtest(evaluation)
    selection = {
        "schema": "QNEXT.INTELLIGENCE.LAB.SELECTION/1",
        "horizon_bars": horizon_bars,
        "selected_algorithm": selected_algorithm,
        "selected_model_hash": selected_hash,
        "ridge_candidate_id": candidate.candidate_id,
        "ml_candidate_id": ml_candidate.candidate_id if ml_candidate is not None else "",
        "ml_family": ml_candidate.family if ml_candidate is not None else "",
        "ml_gate_passed": bool(
            ml_candidate is not None and ml_candidate.promotion_gate.passed
        ),
        "ml_dependencies": dependency_status(),
        "ml_error": ml_error,
        "indicator_feature_counts": indicator_feature_counts,
        "missing_indicators": list(missing_indicators),
        "feature_names": list(dataset.feature_names),
        "evaluated_at_ms": created_at_ms,
    }
    recommendation_record = None
    recommendation_error = ""
    if evaluation.gate_passed:
        try:
            current = build_latest_feature_vector(imported, bars)
            recommendation = build_recommendation(
                experiment_id=experiment_id,
                dataset=dataset,
                historical_examples=examples,
                current=current,
                selected_model=selected_model,
                created_at_ms=created_at_ms,
            )
            recommendation_record = recommendation.to_record()
            registry.save_recommendation_artifact(
                experiment_id,
                recommendation_record,
            )
        except Exception as error:
            recommendation_error = str(error)[:500]

    selection["recommendation_id"] = (
        recommendation_record.get("recommendation_id", "")
        if isinstance(recommendation_record, Mapping)
        else ""
    )
    selection["recommendation_error"] = recommendation_error
    selection_id = registry.save_backtest_selection(experiment_id, selection)

    return {
        "experiment": updated.to_record(),
        "evaluation": evaluation.to_record(),
        "candidate": candidate_record,
        "ml_candidate": ml_record,
        "selected_algorithm": selected_algorithm,
        "selection_id": selection_id,
        "recommendation": recommendation_record,
        "recommendation_error": recommendation_error,
        "ml_dependencies": dependency_status(),
        "ml_error": ml_error,
        "indicator_feature_counts": indicator_feature_counts,
        "missing_indicators": list(missing_indicators),
        "feature_names": list(dataset.feature_names),
    }



def _load_selected_model(
    registry: IntelligenceLabRegistry,
    experiment_id: str,
) -> CandidateModel | LabMLCandidate:
    selection = registry.read_result_record(experiment_id, "selection.json")
    if not isinstance(selection, Mapping):
        raise ValueError("Lab model selection is missing")
    selected_hash = str(selection.get("selected_model_hash", ""))
    if not selected_hash:
        raise ValueError("Lab selected model hash is missing")

    for record in registry.read_candidate_artifacts(experiment_id):
        if str(record.get("model_hash", "")) != selected_hash:
            continue
        if str(record.get("schema", "")) == LAB_ML_SCHEMA:
            return LabMLCandidate.from_record(record)
        return CandidateModel.from_record(record)
    raise ValueError("selected Lab model artifact is missing")


def _shadow_config_from_payload(
    experiment_id: str,
    payload: Mapping[str, Any],
) -> ShadowConfig:
    def number(key: str, default: float) -> float:
        value = payload.get(key, default)
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise ValueError(f"{key} must be numeric")
        result = float(value)
        if not (result >= 0.0 and result < float("inf")):
            raise ValueError(f"{key} must be finite and non-negative")
        return result

    horizon_bars = int(payload.get("horizonBars", 3))
    min_samples = int(payload.get("minSamples", 30))
    if horizon_bars < 1 or horizon_bars > 100:
        raise ValueError("horizonBars must be between 1 and 100")
    if min_samples < 12 or min_samples > 100000:
        raise ValueError("minSamples must be between 12 and 100000")

    return ShadowConfig(
        experiment_id=experiment_id,
        started_at_ms=int(time.time() * 1000),
        horizon_bars=horizon_bars,
        min_samples=min_samples,
        max_accuracy_regression=number("maxAccuracyRegression", 0.05),
        max_average_return_regression=number(
            "maxAverageReturnRegression",
            0.002,
        ),
        max_drawdown_slack=number("maxDrawdownSlack", 0.02),
        max_brier=number("maxBrier", 0.35),
        min_coverage=number("minCoverage", 0.10),
        min_target1_before_invalidation=number(
            "minTarget1BeforeInvalidation",
            0.30,
        ),
    )


def _start_shadow(
    registry: IntelligenceLabRegistry,
    payload: Mapping[str, Any],
) -> dict[str, Any]:
    experiment_id = str(payload.get("experimentId", ""))
    if not experiment_id:
        raise ValueError("experimentId is required")
    experiment = registry.get(experiment_id)
    if experiment.lifecycle_state != "BACKTESTED":
        raise ValueError("Shadow-Live can only start from BACKTESTED")
    recommendation = registry.read_result_record(experiment_id, "recommendation.json")
    selection = registry.read_result_record(experiment_id, "selection.json")
    if not isinstance(recommendation, Mapping):
        raise ValueError("Shadow-Live requires a calibrated Lab recommendation")
    if not isinstance(selection, Mapping):
        raise ValueError("Shadow-Live requires a selected Lab model")
    if not bool(recommendation.get("probability_calibrated", False)):
        raise ValueError("Shadow-Live requires calibrated probabilities")

    default_horizon = int(selection.get("horizon_bars", 3))
    material = dict(payload)
    material.setdefault("horizonBars", default_horizon)
    config = _shadow_config_from_payload(experiment_id, material)
    updated = registry.start_shadow(experiment_id)
    registry.save_shadow_config(experiment_id, config.to_record())
    registry.save_shadow_summary(
        experiment_id,
        summarize_shadow((), ()).to_record(),
    )
    return {
        "experiment": updated.to_record(),
        "shadow_config": config.to_record(),
    }


def _shadow_snapshot_payload(
    registry: IntelligenceLabRegistry,
    experiment_id: str,
    payload: Mapping[str, Any],
) -> Mapping[str, Any]:
    stored = registry.load_chart_snapshot(experiment_id)
    raw = stored.get("payload")
    if not isinstance(raw, Mapping):
        raise ValueError("stored chart snapshot is invalid")

    rows = payload.get("featureRows")
    if not isinstance(rows, list) or not rows or len(rows) > 256:
        raise ValueError("shadow featureRows must contain between 1 and 256 rows")
    current_features = payload.get("currentFeatures", {})
    if not isinstance(current_features, Mapping):
        raise ValueError("shadow currentFeatures must be an object")

    material = dict(raw)
    material["created_at_ms"] = int(payload.get("createdAtMs", time.time() * 1000))
    material["feature_rows"] = rows
    material["current_features"] = dict(current_features)
    return material


def _evaluate_pending_shadow(
    registry: IntelligenceLabRegistry,
    market_core_url: str,
    experiment_id: str,
    config: ShadowConfig,
) -> tuple[tuple[ShadowObservation, ...], tuple[ShadowOutcome, ...]]:
    experiment = registry.get(experiment_id)
    observations = tuple(
        ShadowObservation.from_record(record)
        for record in registry.read_shadow_records(experiment_id, "observations")
    )
    outcomes_by_id = {
        outcome.observation_id: outcome
        for outcome in (
            ShadowOutcome.from_record(record)
            for record in registry.read_shadow_records(experiment_id, "outcomes")
        )
    }
    duration = timeframe_to_milliseconds(experiment.timeframe)
    client = MarketCoreHistoryClient(market_core_url)

    for observation in observations:
        if observation.observation_id in outcomes_by_id:
            continue
        bars = client.fetch_bars(
            instrument_id=experiment.instrument_id,
            timeframe=experiment.timeframe,
            from_ms=observation.as_of_time_ms,
            to_ms=observation.as_of_time_ms
            + duration * (config.horizon_bars + 4),
        )
        future = [
            bar
            for bar in bars
            if bar.open_time_ms > observation.bar_time_ms
            and bar.final
            and bar.quality.upper() in {"GOOD", "RECOVERED"}
        ][: config.horizon_bars]
        if len(future) < config.horizon_bars:
            continue
        outcome = evaluate_shadow_observation(
            observation,
            future_bars=future,
            evaluated_at_ms=max(
                int(time.time() * 1000),
                future[-1].close_time_ms,
            ),
        )
        registry.save_shadow_outcome(experiment_id, outcome.to_record())
        outcomes_by_id[outcome.observation_id] = outcome

    outcomes = tuple(
        sorted(
            outcomes_by_id.values(),
            key=lambda value: (value.evaluated_at_ms, value.observation_id),
        )
    )
    return observations, outcomes


def _shadow_observation(
    registry: IntelligenceLabRegistry,
    market_core_url: str,
    payload: Mapping[str, Any],
) -> dict[str, Any]:
    experiment_id = str(payload.get("experimentId", ""))
    if not experiment_id:
        raise ValueError("experimentId is required")
    experiment = registry.get(experiment_id)
    if experiment.lifecycle_state != "SHADOW":
        raise ValueError("shadow observations require SHADOW state")

    config_record = registry.read_shadow_config(experiment_id)
    if not isinstance(config_record, Mapping):
        raise ValueError("Shadow-Live config is missing")
    config = ShadowConfig.from_record(config_record)

    configuration_hash = str(payload.get("indicatorConfigurationHash", ""))
    if configuration_hash != experiment.indicator_configuration_hash:
        raise ValueError("shadow chart indicator configuration does not match experiment")
    feature_schema_version = str(payload.get("featureSchemaVersion", ""))
    if feature_schema_version != experiment.feature_schema_version:
        raise ValueError("shadow chart feature schema does not match experiment")

    snapshot_payload = _shadow_snapshot_payload(
        registry,
        experiment_id,
        payload,
    )
    imported = import_chart_snapshot(snapshot_payload)
    if imported.instrument_id != experiment.instrument_id:
        raise ValueError("shadow observation instrument does not match experiment")
    if imported.timeframe != experiment.timeframe:
        raise ValueError("shadow observation timeframe does not match experiment")

    expected_bar_time = int(payload.get("barTimeMs", 0))
    if expected_bar_time <= 0:
        raise ValueError("barTimeMs is required")
    duration = timeframe_to_milliseconds(experiment.timeframe)
    first_time = imported.feature_rows[0].bar_time_ms
    client = MarketCoreHistoryClient(market_core_url)
    bars = client.fetch_bars(
        instrument_id=experiment.instrument_id,
        timeframe=experiment.timeframe,
        from_ms=max(0, first_time),
        to_ms=expected_bar_time + duration * 2,
    )
    current = build_latest_feature_vector(imported, bars)
    if current.bar_time_ms != expected_bar_time:
        raise ValueError("shadow feature vector is not aligned to the requested finalized bar")

    selected_model = _load_selected_model(registry, experiment_id)
    recommendation = registry.read_result_record(experiment_id, "recommendation.json")
    if not isinstance(recommendation, Mapping):
        raise ValueError("shadow recommendation policy is missing")

    observation = score_shadow_observation(
        experiment_id=experiment_id,
        current=current,
        selected_model=selected_model,
        recommendation_policy=recommendation,
        created_at_ms=int(payload.get("createdAtMs", time.time() * 1000)),
    )
    registry.save_shadow_observation(
        experiment_id,
        observation.to_record(),
    )
    observations, outcomes = _evaluate_pending_shadow(
        registry,
        market_core_url,
        experiment_id,
        config,
    )
    summary = summarize_shadow(observations, outcomes)
    registry.save_shadow_summary(experiment_id, summary.to_record())
    return {
        "observation": observation.to_record(),
        "summary": summary.to_record(),
    }


def _certify_shadow(
    registry: IntelligenceLabRegistry,
    market_core_url: str,
    payload: Mapping[str, Any],
) -> dict[str, Any]:
    experiment_id = str(payload.get("experimentId", ""))
    if not experiment_id:
        raise ValueError("experimentId is required")
    experiment = registry.get(experiment_id)
    if experiment.lifecycle_state != "SHADOW":
        raise ValueError("certification requires SHADOW state")

    config_record = registry.read_shadow_config(experiment_id)
    if not isinstance(config_record, Mapping):
        raise ValueError("Shadow-Live config is missing")
    config = ShadowConfig.from_record(config_record)
    observations, outcomes = _evaluate_pending_shadow(
        registry,
        market_core_url,
        experiment_id,
        config,
    )
    summary = summarize_shadow(observations, outcomes)
    registry.save_shadow_summary(experiment_id, summary.to_record())

    backtest = registry.read_evaluation(experiment_id, "BACKTEST")
    selection = registry.read_result_record(experiment_id, "selection.json")
    if not isinstance(backtest, Mapping) or not isinstance(selection, Mapping):
        raise ValueError("Shadow certification requires backtest and model selection")
    backtest_metrics = backtest.get("metrics")
    if not isinstance(backtest_metrics, Mapping):
        raise ValueError("backtest metrics are invalid")

    passed, reasons = certification_gate(
        summary,
        backtest_metrics=backtest_metrics,
        config=config,
    )
    evidence_hash = stable_hash({
        "observations": [value.to_record() for value in observations],
        "outcomes": [value.to_record() for value in outcomes],
        "summary": summary.to_record(),
    })
    model_hash = str(selection.get("selected_model_hash", ""))
    evaluation = LabEvaluation(
        experiment_id=experiment_id,
        phase="SHADOW",
        evaluated_at_ms=int(time.time() * 1000),
        dataset_hash=evidence_hash,
        model_spec_hash=model_hash,
        gate_passed=passed,
        metrics={
            "samples": float(summary.metrics.samples),
            "accuracy": summary.metrics.accuracy,
            "coverage": summary.metrics.coverage,
            "strategy_return": summary.metrics.strategy_return,
            "average_strategy_return": summary.metrics.average_strategy_return,
            "max_drawdown": summary.metrics.max_drawdown,
            "trades": float(summary.metrics.trades),
            "average_brier": summary.average_brier,
            "target1_samples": float(summary.target1_samples),
            "target1_before_invalidation": summary.target1_before_invalidation,
            "target2_before_invalidation": summary.target2_before_invalidation,
            "invalidation_before_target1": summary.invalidation_before_target1,
            "pending_samples": float(summary.pending_samples),
        },
        reasons=reasons,
    )
    updated = registry.certify(evaluation)
    return {
        "experiment": updated.to_record(),
        "evaluation": evaluation.to_record(),
        "summary": summary.to_record(),
    }


def process_request(
    request: Mapping[str, Any],
    *,
    storage_root: Path,
    production_root: Path,
    market_core_url: str,
) -> dict[str, Any]:
    if request.get("schema") != REQUEST_SCHEMA:
        raise ValueError("unsupported intelligence lab request schema")
    request_id = str(request.get("request_id", ""))
    action = str(request.get("action", ""))
    payload = request.get("payload")
    if not request_id or not isinstance(payload, Mapping):
        raise ValueError("invalid intelligence lab request")

    registry = IntelligenceLabRegistry(storage_root, production_root=production_root)
    if action == "import":
        result = _import_snapshot(registry, payload)
    elif action == "backtest":
        result = _backtest(registry, market_core_url, payload)
    elif action == "start-shadow":
        result = _start_shadow(registry, payload)
    elif action == "shadow-observation":
        result = _shadow_observation(registry, market_core_url, payload)
    elif action == "certify-shadow":
        result = _certify_shadow(registry, market_core_url, payload)
    else:
        raise ValueError("unsupported intelligence lab action")

    return {
        "state": "SUCCESS",
        "request_id": request_id,
        "action": action,
        "completed_at_ms": int(time.time() * 1000),
        "result": result,
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Process one queued QNext Intelligence Lab action")
    parser.add_argument("--request", required=True)
    parser.add_argument("--result", required=True)
    parser.add_argument("--storage-root", required=True)
    parser.add_argument("--production-root", required=True)
    parser.add_argument("--market-core-url", required=True)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    request_path = Path(args.request)
    result_path = Path(args.result)
    request: dict[str, Any] = {}
    try:
        with request_path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        if not isinstance(decoded, dict):
            raise ValueError("request must be an object")
        request = decoded
        output = process_request(
            request,
            storage_root=Path(args.storage_root),
            production_root=Path(args.production_root),
            market_core_url=args.market_core_url,
        )
        _write_json(result_path, output)
        return 0
    except Exception as error:
        output = {
            "state": "FAILED",
            "request_id": str(request.get("request_id", "")),
            "action": str(request.get("action", "")),
            "completed_at_ms": int(time.time() * 1000),
            "error": str(error)[:1000],
        }
        _write_json(result_path, output)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
