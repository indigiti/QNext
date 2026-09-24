from __future__ import annotations

from math import sqrt
from statistics import fmean
from typing import Sequence

from .domain import Bar, FeatureVector, stable_hash

FEATURE_SET_VERSION = "qnext.features.v1"
_ALLOWED_QUALITY = {"GOOD", "RECOVERED"}


def _returns(closes: Sequence[float]) -> list[float]:
    return [(closes[i] / closes[i - 1]) - 1.0 for i in range(1, len(closes))]


def _std(values: Sequence[float]) -> float:
    if len(values) < 2:
        return 0.0
    mean = fmean(values)
    return sqrt(sum((value - mean) ** 2 for value in values) / len(values))


def _ema(values: Sequence[float], period: int) -> float:
    alpha = 2.0 / (period + 1.0)
    result = values[0]
    for value in values[1:]:
        result = alpha * value + (1.0 - alpha) * result
    return result


def build_feature_vector(bars: Sequence[Bar], *, lookback: int = 20) -> FeatureVector:
    if lookback < 5:
        raise ValueError("lookback must be >= 5")
    if len(bars) < lookback + 1:
        raise ValueError("insufficient finalized bars for feature window")

    window = list(bars[-(lookback + 1):])
    instrument_ids = {bar.instrument_id for bar in window}
    timeframes = {bar.timeframe for bar in window}
    if len(instrument_ids) != 1 or len(timeframes) != 1:
        raise ValueError("feature window must contain one instrument/timeframe")
    if any(not bar.final for bar in window):
        raise ValueError("features may only use finalized bars")
    if any(window[i].time_ms >= window[i + 1].time_ms for i in range(len(window) - 1)):
        raise ValueError("bars must be strictly increasing by event time")

    closes = [bar.close for bar in window]
    recent = closes[-lookback:]
    returns = _returns(closes)
    current = window[-1]
    previous = window[-2]
    high_20 = max(bar.high for bar in window[-lookback:])
    low_20 = min(bar.low for bar in window[-lookback:])
    range_width = max(high_20 - low_20, 1e-12)

    features = {
        "return_1": returns[-1],
        "return_3": (current.close / window[-4].close) - 1.0,
        "return_5": (current.close / window[-6].close) - 1.0,
        "ema_5_distance": (current.close / _ema(recent[-5:], 5)) - 1.0,
        "ema_20_distance": (current.close / _ema(recent, 20)) - 1.0,
        "volatility_20": _std(returns[-lookback:]),
        "range_position_20": (current.close - low_20) / range_width,
        "bar_range_pct": (current.high - current.low) / max(abs(current.close), 1e-12),
        "body_pct": (current.close - current.open) / max(abs(current.open), 1e-12),
        "volume_change": 0.0 if previous.volume <= 0 else (current.volume / previous.volume) - 1.0,
    }

    data_quality = "GOOD" if all(bar.quality in _ALLOWED_QUALITY for bar in window) else "DEGRADED"
    snapshot_payload = {
        "feature_set_version": FEATURE_SET_VERSION,
        "instrument_id": current.instrument_id,
        "timeframe": current.timeframe,
        "as_of_time_ms": current.time_ms,
        "features": features,
        "source_bar_keys": [f"{bar.instrument_id}|{bar.timeframe}|{bar.time_ms}|r{bar.revision}" for bar in window],
    }
    return FeatureVector(
        feature_set_version=FEATURE_SET_VERSION,
        instrument_id=current.instrument_id,
        timeframe=current.timeframe,
        as_of_time_ms=current.time_ms,
        features=features,
        snapshot_hash=stable_hash(snapshot_payload),
        data_quality=data_quality,
    )
