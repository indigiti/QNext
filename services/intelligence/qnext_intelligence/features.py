from __future__ import annotations

import math
from statistics import pstdev
from typing import Sequence

from .domain import Bar, FeatureVector, stable_hash

FEATURE_SET_VERSION = "qnext-basic-v1"


def _quality(bars: Sequence[Bar]) -> str:
    qualities = {bar.quality.upper() for bar in bars}
    if qualities <= {"GOOD"}:
        return "GOOD"
    if "INVALID" in qualities:
        return "INVALID"
    if "STALE" in qualities:
        return "STALE"
    if "DEGRADED" in qualities:
        return "DEGRADED"
    if "PARTIAL" in qualities:
        return "PARTIAL"
    if "RECOVERED" in qualities:
        return "RECOVERED"
    return "DEGRADED"


def build_feature_vector(
    bars: Sequence[Bar],
    *,
    as_of_time_ms: int,
    lookback: int = 5,
    feature_set_version: str = FEATURE_SET_VERSION,
) -> FeatureVector:
    if lookback < 3:
        raise ValueError("lookback must be >= 3")
    if len(bars) < lookback:
        raise ValueError("insufficient bars for requested lookback")

    ordered = sorted(bars, key=lambda bar: bar.close_time_ms)
    eligible = [bar for bar in ordered if bar.final and bar.close_time_ms <= as_of_time_ms]
    if len(eligible) < lookback:
        raise ValueError("insufficient finalized bars available at as_of_time_ms")

    selected = eligible[-lookback:]
    instrument_ids = {bar.instrument_id for bar in selected}
    timeframes = {bar.timeframe for bar in selected}
    if len(instrument_ids) != 1 or len(timeframes) != 1:
        raise ValueError("feature window must contain one instrument and one timeframe")
    for bar in selected:
        bar.validate()

    closes = [bar.close for bar in selected]
    returns = [
        (closes[i] / closes[i - 1]) - 1.0
        for i in range(1, len(closes))
        if closes[i - 1] != 0
    ]
    if len(returns) != lookback - 1:
        raise ValueError("zero close is not valid for return features")

    sma = sum(closes) / len(closes)
    last = selected[-1]
    true_range = last.high - last.low
    features = {
        "return_1": returns[-1],
        "return_window": (closes[-1] / closes[0]) - 1.0,
        "volatility_window": pstdev(returns) if len(returns) > 1 else 0.0,
        "close_vs_sma": (closes[-1] / sma) - 1.0 if sma else 0.0,
        "range_pct": true_range / last.close if last.close else 0.0,
        "volume_log1p": math.log1p(max(last.volume, 0.0)),
    }

    snapshot_material = {
        "feature_set_version": feature_set_version,
        "instrument_id": last.instrument_id,
        "timeframe": last.timeframe,
        "as_of_time_ms": as_of_time_ms,
        "bars": [
            {
                "open_time_ms": bar.open_time_ms,
                "close_time_ms": bar.close_time_ms,
                "open": bar.open,
                "high": bar.high,
                "low": bar.low,
                "close": bar.close,
                "volume": bar.volume,
                "revision": bar.revision,
                "quality": bar.quality,
            }
            for bar in selected
        ],
        "features": features,
    }

    return FeatureVector(
        feature_set_version=feature_set_version,
        instrument_id=last.instrument_id,
        timeframe=last.timeframe,
        as_of_time_ms=as_of_time_ms,
        features=features,
        snapshot_hash=stable_hash(snapshot_material),
        data_quality=_quality(selected),
    )
