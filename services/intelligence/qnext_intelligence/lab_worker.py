from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
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


def _common_feature_names(examples: Sequence[HistoricalLabExample]) -> tuple[str, ...]:
    if not examples:
        return ()
    names = set(examples[0].features)
    for example in examples[1:]:
        names.intersection_update(example.features)
    return tuple(sorted(names))


def _learning_records(
    experiment_id: str,
    feature_schema_version: str,
    examples: Sequence[HistoricalLabExample],
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[dict[str, Any]]]:
    names = _common_feature_names(examples)
    if not names:
        raise ValueError("historical examples have no common indicator features")

    features: list[dict[str, Any]] = []
    predictions: list[dict[str, Any]] = []
    outcomes: list[dict[str, Any]] = []

    for index, example in enumerate(examples):
        values = {name: float(example.features[name]) for name in names}
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
    )
    dataset = build_learning_dataset(
        feature_records,
        prediction_records,
        outcome_records,
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

    metrics = {
        "samples": float(candidate.test_metrics.samples),
        "accuracy": candidate.test_metrics.accuracy,
        "coverage": candidate.test_metrics.coverage,
        "strategy_return": candidate.test_metrics.strategy_return,
        "average_strategy_return": candidate.test_metrics.average_strategy_return,
        "max_drawdown": candidate.test_metrics.max_drawdown,
        "trades": float(candidate.test_metrics.trades),
        "champion_accuracy": candidate.champion_test_metrics.accuracy,
        "champion_average_strategy_return": candidate.champion_test_metrics.average_strategy_return,
        "champion_max_drawdown": candidate.champion_test_metrics.max_drawdown,
        "historical_examples": float(len(examples)),
        "feature_count": float(len(dataset.feature_names)),
    }
    evaluation = LabEvaluation(
        experiment_id=experiment_id,
        phase="BACKTEST",
        evaluated_at_ms=created_at_ms,
        dataset_hash=dataset_hash,
        model_spec_hash=candidate.model_hash,
        gate_passed=candidate.promotion_gate.passed,
        metrics=metrics,
        reasons=candidate.promotion_gate.reasons,
    )
    updated = registry.record_backtest(evaluation)

    return {
        "experiment": updated.to_record(),
        "evaluation": evaluation.to_record(),
        "candidate": candidate_record,
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
