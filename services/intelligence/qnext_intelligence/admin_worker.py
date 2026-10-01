from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import time
from typing import Any

from .learning import build_learning_dataset, train_candidate
from .registry import ModelRegistry
from .store import ImmutableJSONLStore


def _records(root: Path, name: str, identity_field: str) -> list[dict[str, Any]]:
    return ImmutableJSONLStore(root / name, identity_field=identity_field).read_all()


def _write_json(path: Path, value: dict[str, Any]) -> None:
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


def _train(root: Path, payload: dict[str, Any]) -> dict[str, Any]:
    dataset = build_learning_dataset(
        _records(root, "features.jsonl", "snapshot_hash"),
        _records(root, "predictions.jsonl", "prediction_id"),
        _records(root, "outcomes.jsonl", "prediction_id"),
    )
    candidate = train_candidate(
        dataset,
        model_name="qnext-self-learning-direction",
        created_at_ms=int(time.time() * 1000),
        min_samples=int(payload.get("minSamples", 60)),
        min_test_samples=int(payload.get("minTestSamples", 12)),
        min_average_return_improvement=float(payload.get("minAverageReturnImprovement", 0.0)),
        max_accuracy_regression=float(payload.get("maxAccuracyRegression", 0.02)),
        max_drawdown_slack=float(payload.get("maxDrawdownSlack", 0.01)),
    )
    ModelRegistry(root / "models").save_candidate(candidate)
    return candidate.to_record()


def _promote(root: Path, payload: dict[str, Any]) -> dict[str, Any]:
    return ModelRegistry(root / "models").promote(
        str(payload["candidateId"]),
        approved_by=str(payload["approvedBy"]),
        expected_model_hash=str(payload["expectedModelHash"]),
        promoted_at_ms=int(time.time() * 1000),
        note=str(payload.get("note", "")),
    )


def _rollback(root: Path, payload: dict[str, Any]) -> dict[str, Any]:
    candidate = payload.get("candidateId")
    return ModelRegistry(root / "models").rollback(
        approved_by=str(payload["approvedBy"]),
        event_at_ms=int(time.time() * 1000),
        candidate_id=str(candidate) if candidate else None,
        note=str(payload.get("note", "")),
    )


def process_request(request: dict[str, Any], storage_root: Path) -> dict[str, Any]:
    if request.get("schema") != "QNEXT.INTELLIGENCE.REQUEST/1":
        raise ValueError("unsupported request schema")
    request_id = str(request.get("request_id", ""))
    action = str(request.get("action", ""))
    payload = request.get("payload")
    if not request_id or not isinstance(payload, dict):
        raise ValueError("invalid request")
    if action == "train":
        result = _train(storage_root, payload)
    elif action == "promote":
        result = _promote(storage_root, payload)
    elif action == "rollback":
        result = _rollback(storage_root, payload)
    else:
        raise ValueError("unsupported action")
    return {
        "state": "SUCCESS",
        "request_id": request_id,
        "action": action,
        "completed_at_ms": int(time.time() * 1000),
        "result": result,
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Process one queued QNext Intelligence admin action")
    parser.add_argument("--request", required=True)
    parser.add_argument("--result", required=True)
    parser.add_argument("--storage-root", required=True)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    request_path = Path(args.request)
    result_path = Path(args.result)
    storage_root = Path(args.storage_root)
    request: dict[str, Any] = {}
    try:
        with request_path.open("r", encoding="utf-8") as handle:
            decoded = json.load(handle)
        if not isinstance(decoded, dict):
            raise ValueError("request must be an object")
        request = decoded
        output = process_request(request, storage_root)
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
