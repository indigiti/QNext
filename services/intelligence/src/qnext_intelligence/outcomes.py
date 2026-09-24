from __future__ import annotations

from typing import Sequence

from .domain import Bar, Outcome, Prediction


def evaluate_outcome(prediction: Prediction, bars: Sequence[Bar], *, flat_threshold: float = 0.0005) -> Outcome:
    future = [
        bar
        for bar in bars
        if bar.instrument_id == prediction.instrument_id
        and bar.timeframe == prediction.timeframe
        and bar.final
        and bar.time_ms > prediction.prediction_time_ms
    ]
    if len(future) < prediction.horizon_bars:
        raise ValueError("prediction horizon is not complete")

    horizon = future[:prediction.horizon_bars]
    entry_candidates = [
        bar
        for bar in bars
        if bar.instrument_id == prediction.instrument_id
        and bar.timeframe == prediction.timeframe
        and bar.final
        and bar.time_ms == prediction.prediction_time_ms
    ]
    if not entry_candidates:
        raise ValueError("prediction source bar is unavailable")

    entry = entry_candidates[-1].close
    final_close = horizon[-1].close
    return_value = (final_close / entry) - 1.0
    mfe = (max(bar.high for bar in horizon) / entry) - 1.0
    mae = (min(bar.low for bar in horizon) / entry) - 1.0
    if return_value > flat_threshold:
        actual = "UP"
    elif return_value < -flat_threshold:
        actual = "DOWN"
    else:
        actual = "FLAT"

    return Outcome(
        prediction_id=prediction.prediction_id,
        return_value=return_value,
        mfe=mfe,
        mae=mae,
        direction_actual=actual,
        correct=prediction.direction == actual,
        evaluated_at_ms=horizon[-1].time_ms,
    )
