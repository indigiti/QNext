from __future__ import annotations

import json
from typing import Callable
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from .domain import Bar


def timeframe_to_milliseconds(timeframe: str) -> int:
    value = timeframe.strip().lower()
    if len(value) < 2:
        raise ValueError("invalid timeframe")
    try:
        amount = int(value[:-1])
    except ValueError as exc:
        raise ValueError("invalid timeframe") from exc
    if amount <= 0:
        raise ValueError("timeframe amount must be positive")

    unit = value[-1]
    multipliers = {
        "s": 1_000,
        "m": 60_000,
        "h": 3_600_000,
        "d": 86_400_000,
    }
    if unit not in multipliers:
        raise ValueError("unsupported timeframe unit")
    return amount * multipliers[unit]


class MarketCoreHistoryClient:
    def __init__(
        self,
        base_url: str,
        *,
        timeout: float = 10.0,
        opener: Callable[..., object] | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        if not self.base_url:
            raise ValueError("market core base URL is required")
        if timeout <= 0:
            raise ValueError("timeout must be positive")
        self.timeout = timeout
        self._opener = opener or urlopen

    def fetch_bars(
        self,
        *,
        instrument_id: str,
        timeframe: str,
        from_ms: int,
        to_ms: int,
    ) -> list[Bar]:
        if not instrument_id:
            raise ValueError("instrument_id is required")
        if from_ms >= to_ms:
            raise ValueError("from_ms must be less than to_ms")
        duration_ms = timeframe_to_milliseconds(timeframe)

        query = urlencode(
            {
                "instrument_id": instrument_id,
                "timeframe": timeframe,
                "from_ms": from_ms,
                "to_ms": to_ms,
            }
        )
        request = Request(
            f"{self.base_url}/api/v1/bars?{query}",
            headers={"Accept": "application/json"},
            method="GET",
        )
        response = self._opener(request, timeout=self.timeout)
        try:
            payload = json.loads(response.read().decode("utf-8"))
        finally:
            close = getattr(response, "close", None)
            if close is not None:
                close()

        raw_bars = payload.get("bars") if isinstance(payload, dict) else None
        if not isinstance(raw_bars, list):
            raise ValueError("invalid Market Core bars response")

        bars: list[Bar] = []
        previous_open_ms: int | None = None
        for raw in raw_bars:
            if not isinstance(raw, dict):
                raise ValueError("invalid bar record")
            try:
                open_time_ms = int(raw["time"])
                bar = Bar(
                    instrument_id=instrument_id,
                    timeframe=timeframe,
                    open_time_ms=open_time_ms,
                    close_time_ms=open_time_ms + duration_ms,
                    open=float(raw["open"]),
                    high=float(raw["high"]),
                    low=float(raw["low"]),
                    close=float(raw["close"]),
                    volume=float(raw.get("volume", 0.0)),
                    final=bool(raw["final"]),
                    revision=int(raw.get("revision", 0)),
                    quality=str(raw.get("quality", "DEGRADED")),
                    authority_provider=str(raw.get("authority_provider", "")),
                )
            except (KeyError, TypeError, ValueError) as exc:
                raise ValueError("invalid Market Core bar record") from exc

            if previous_open_ms is not None and open_time_ms <= previous_open_ms:
                raise ValueError("Market Core bars must be strictly increasing")
            bar.validate()
            bars.append(bar)
            previous_open_ms = open_time_ms

        return bars
