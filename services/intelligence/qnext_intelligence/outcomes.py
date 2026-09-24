from __future__ import annotations

import re
from pathlib import Path

from .models import Outcome, Prediction, VALID_DIRECTIONS
from .storage import AppendOnlyJsonlStore


_TIMEFRAME = re.compile(r"^(\d+)(s|m|h)$")


class OutcomeError(ValueError):
    pass


def timeframe_ms(value: str) -> int:
    match = _TIMEFRAME.fullmatch(value.strip())
    if not match:
        raise OutcomeError(f"unsupported timeframe {value!r}")
    count = int(match.group(1))
    unit = match.group(2)
    multiplier = {"s": 1_000, "m": 60_000, "h": 3_600_000}[unit]
    return count * multiplier


class OutcomeService:
    def __init__(self, root: str | Path) -> None:
        self._store = AppendOnlyJsonlStore(Path(root) / "outcomes" / "outcomes.jsonl", "prediction_id")

    def evaluate(
        self,
        *,
        prediction: Prediction,
        entry_price: float,
        exit_price: float,
        evaluated_at_ms: int,
        mfe: float = 0.0,
        mae: float = 0.0,
    ) -> Outcome:
        if entry_price <= 0 or exit_price <= 0:
            raise OutcomeError("entry and exit prices must be positive")
        earliest = prediction.prediction_time_ms + timeframe_ms(prediction.timeframe) * prediction.horizon_bars
        if evaluated_at_ms < earliest:
            raise OutcomeError("outcome cannot be evaluated before the prediction horizon closes")
        realized = (exit_price - entry_price) / entry_price
        actual = "UP" if realized > 0 else "DOWN" if realized < 0 else "FLAT"
        if actual not in VALID_DIRECTIONS:
            raise OutcomeError("invalid derived direction")
        outcome = Outcome(
            prediction_id=prediction.prediction_id,
            return_value=realized,
            mfe=float(mfe),
            mae=float(mae),
            direction_actual=actual,
            correct=prediction.direction == actual,
            evaluated_at_ms=evaluated_at_ms,
        )
        self._store.append(outcome.to_record())
        return outcome

    def get(self, prediction_id: str) -> dict[str, object] | None:
        return self._store.get(prediction_id)
