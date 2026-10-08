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
    historical_dataset_hash,
    import_chart_snapshot,
)
from .learning import build_learning_dataset, train_candidate
from .lab_ml import dependency_status, train_ml_challenger

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

    selected_algorithm = candidate.algorithm
    selected_hash = candidate.model_hash
    selected_metrics = candidate.test_metrics
    selected_gate = candidate.promotion_gate
    if ml_candidate is not None and ml_candidate.promotion_gate.passed:
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

    return {
        "experiment": updated.to_record(),
        "evaluation": evaluation.to_record(),
        "candidate": candidate_record,
        "ml_candidate": ml_record,
        "selected_algorithm": selected_algorithm,
        "ml_dependencies": dependency_status(),
        "ml_error": ml_error,
        "indicator_feature_counts": indicator_feature_counts,
        "missing_indicators": list(missing_indicators),
        "feature_names": list(dataset.feature_names),
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
