from __future__ import annotations

import argparse
import json
from pathlib import Path
import time

from .learning import build_learning_dataset, train_candidate
from .registry import ModelRegistry
from .store import ImmutableJSONLStore


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="QNext controlled self-learning workflow")
    sub = parser.add_subparsers(dest="command", required=True)

    train = sub.add_parser(
        "train",
        help="analyze outcomes, optimize on validation, backtest on untouched test data, and write a DRAFT candidate",
    )
    train.add_argument("--storage-root", required=True)
    train.add_argument("--model-name", default="qnext-self-learning-direction")
    train.add_argument("--created-at-ms", type=int, default=0)
    train.add_argument("--min-samples", type=int, default=60)
    train.add_argument("--min-test-samples", type=int, default=12)
    train.add_argument("--min-average-return-improvement", type=float, default=0.0)
    train.add_argument("--max-accuracy-regression", type=float, default=0.02)
    train.add_argument("--max-drawdown-slack", type=float, default=0.01)

    status = sub.add_parser("status", help="show DRAFT candidates and current production pointer")
    status.add_argument("--storage-root", required=True)

    promote = sub.add_parser("promote", help="explicitly promote a certified DRAFT candidate")
    promote.add_argument("--storage-root", required=True)
    promote.add_argument("--candidate-id", required=True)
    promote.add_argument("--expected-model-hash", required=True)
    promote.add_argument("--approved-by", required=True)
    promote.add_argument("--event-at-ms", type=int, default=0)
    promote.add_argument("--note", default="")

    rollback = sub.add_parser("rollback", help="explicitly roll production back to a prior certified candidate")
    rollback.add_argument("--storage-root", required=True)
    rollback.add_argument("--candidate-id")
    rollback.add_argument("--approved-by", required=True)
    rollback.add_argument("--event-at-ms", type=int, default=0)
    rollback.add_argument("--note", default="")
    return parser


def _records(root: Path, name: str, identity_field: str) -> list[dict]:
    return ImmutableJSONLStore(root / name, identity_field=identity_field).read_all()


def run_train(args: argparse.Namespace) -> dict:
    root = Path(args.storage_root)
    dataset = build_learning_dataset(
        _records(root, "features.jsonl", "snapshot_hash"),
        _records(root, "predictions.jsonl", "prediction_id"),
        _records(root, "outcomes.jsonl", "prediction_id"),
    )
    created_at_ms = args.created_at_ms or int(time.time() * 1000)
    candidate = train_candidate(
        dataset,
        model_name=args.model_name,
        created_at_ms=created_at_ms,
        min_samples=args.min_samples,
        min_test_samples=args.min_test_samples,
        min_average_return_improvement=args.min_average_return_improvement,
        max_accuracy_regression=args.max_accuracy_regression,
        max_drawdown_slack=args.max_drawdown_slack,
    )
    ModelRegistry(root / "models").save_candidate(candidate)
    return candidate.to_record()


def run_status(args: argparse.Namespace) -> dict:
    registry = ModelRegistry(Path(args.storage_root) / "models")
    return {"production": registry.production(), "candidates": registry.list_candidates()}


def run_promote(args: argparse.Namespace) -> dict:
    registry = ModelRegistry(Path(args.storage_root) / "models")
    event_at_ms = args.event_at_ms or int(time.time() * 1000)
    return registry.promote(
        args.candidate_id,
        approved_by=args.approved_by,
        expected_model_hash=args.expected_model_hash,
        promoted_at_ms=event_at_ms,
        note=args.note,
    )


def run_rollback(args: argparse.Namespace) -> dict:
    registry = ModelRegistry(Path(args.storage_root) / "models")
    event_at_ms = args.event_at_ms or int(time.time() * 1000)
    return registry.rollback(
        approved_by=args.approved_by,
        event_at_ms=event_at_ms,
        candidate_id=args.candidate_id,
        note=args.note,
    )


def run(args: argparse.Namespace) -> dict:
    if args.command == "train":
        return run_train(args)
    if args.command == "status":
        return run_status(args)
    if args.command == "promote":
        return run_promote(args)
    if args.command == "rollback":
        return run_rollback(args)
    raise ValueError(f"unsupported command: {args.command}")


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    print(json.dumps(run(args), sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
