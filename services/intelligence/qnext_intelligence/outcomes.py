from __future__ import annotations

from typing import Sequence

from .domain import Bar, Outcome, Prediction


def evaluate_outcome(
    prediction: Prediction,
    *,
    origin_close: float,
    future_bars: Sequence[Bar],
) -> Outcome:
    if prediction.state != "IMMUTABLE":
        raise ValueError("withheld prediction cannot be outcome-evaluated")
    if origin_close <= 0:
        raise ValueError("origin_close must be positive")
    eligible = sorted(
        (
            bar
            for bar in future_bars
            if bar.final
            and bar.instrument_id == prediction.instrument_id
            and bar.timeframe == prediction.timeframe
            and bar.close_time_ms > prediction.prediction_time_ms
        ),
        key=lambda bar: bar.close_time_ms,
    )
    if len(eligible) < prediction.horizon_bars:
        raise ValueError("prediction horizon is not complete")

    window = eligible[: prediction.horizon_bars]
    for bar in window:
        bar.validate()

    terminal = window[-1].close
    return_value = (terminal / origin_close) - 1.0
    mfe = max((bar.high / origin_close) - 1.0 for bar in window)
    mae = min((bar.low / origin_close) - 1.0 for bar in window)

    if terminal > origin_close:
        actual = "UP"
    elif terminal < origin_close:
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
        evaluated_at_ms=window[-1].close_time_ms,
    )
