from __future__ import annotations

import argparse
import json
from pathlib import Path

from .baseline import baseline_manifest, baseline_probabilities, classify_regime
from .client import MarketCoreHistoryClient
from .features import build_feature_vector
from .prediction import predict
from .store import ImmutableJSONLStore


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Run QNext deterministic intelligence")
    parser.add_argument("--market-core-url", required=True)
    parser.add_argument("--instrument-id", required=True)
    parser.add_argument("--timeframe", required=True)
    parser.add_argument("--from-ms", type=int, required=True)
    parser.add_argument("--to-ms", type=int, required=True)
    parser.add_argument("--storage-root", required=True)
    parser.add_argument("--lookback", type=int, default=20)
    parser.add_argument("--horizon-bars", type=int, default=3)
    parser.add_argument("--calendar-version", default="UNSPECIFIED")
    parser.add_argument("--session", default="UNSPECIFIED")
    parser.add_argument("--strategy-version", default="UNSPECIFIED")
    parser.add_argument("--configuration-hash", default="UNSPECIFIED")
    return parser


def run(args: argparse.Namespace) -> dict:
    client = MarketCoreHistoryClient(args.market_core_url)
    bars = client.fetch_bars(
        instrument_id=args.instrument_id,
        timeframe=args.timeframe,
        from_ms=args.from_ms,
        to_ms=args.to_ms,
    )
    finalized = [bar for bar in bars if bar.final]
    if not finalized:
        raise ValueError("no finalized bars returned by Market Core")

    as_of_time_ms = finalized[-1].close_time_ms
    features = build_feature_vector(
        bars,
        as_of_time_ms=as_of_time_ms,
        lookback=args.lookback,
        calendar_version=getattr(args, "calendar_version", "UNSPECIFIED"),
        session=getattr(args, "session", "UNSPECIFIED"),
        configuration_hash=getattr(args, "configuration_hash", "UNSPECIFIED"),
    )
    manifest = baseline_manifest(features.feature_set_version, as_of_time_ms)
    prediction = predict(
        features,
        manifest,
        baseline_probabilities,
        horizon_bars=args.horizon_bars,
        regime=classify_regime(features),
        strategy_version=getattr(args, "strategy_version", "UNSPECIFIED"),
    )

    root = Path(args.storage_root)
    ImmutableJSONLStore(root / "features.jsonl", identity_field="snapshot_hash").append(
        features.to_record()
    )
    ImmutableJSONLStore(root / "predictions.jsonl", identity_field="prediction_id").append(
        prediction.to_record()
    )
    return prediction.to_record()


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    result = run(args)
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
