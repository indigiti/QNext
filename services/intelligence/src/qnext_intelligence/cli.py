from __future__ import annotations

import argparse
import json
import sys

from .client import MarketCoreClient
from .engine import IntelligenceEngine


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Run one QNext intelligence inference from canonical Market Core bars")
    parser.add_argument("--market-core-url", required=True)
    parser.add_argument("--instrument-id", required=True)
    parser.add_argument("--timeframe", required=True)
    parser.add_argument("--from-ms", type=int, required=True)
    parser.add_argument("--to-ms", type=int, required=True)
    parser.add_argument("--storage-root", required=True)
    parser.add_argument("--lookback", type=int, default=20)
    parser.add_argument("--horizon-bars", type=int, default=3)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    client = MarketCoreClient(args.market_core_url)
    bars = client.bars(
        instrument_id=args.instrument_id,
        timeframe=args.timeframe,
        from_ms=args.from_ms,
        to_ms=args.to_ms,
    )
    engine = IntelligenceEngine(args.storage_root)
    vector, prediction = engine.infer(bars, lookback=args.lookback, horizon_bars=args.horizon_bars)
    print(json.dumps({"feature_vector": vector.to_dict(), "prediction": prediction.to_dict()}, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
