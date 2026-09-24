from __future__ import annotations

import json
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from .domain import Bar


class MarketCoreClient:
    def __init__(self, base_url: str, *, timeout_seconds: float = 5.0):
        self.base_url = base_url.rstrip("/")
        self.timeout_seconds = timeout_seconds

    def bars(self, *, instrument_id: str, timeframe: str, from_ms: int, to_ms: int) -> list[Bar]:
        query = urlencode(
            {
                "instrument_id": instrument_id,
                "timeframe": timeframe,
                "from_ms": from_ms,
                "to_ms": to_ms,
            }
        )
        request = Request(f"{self.base_url}/api/v1/bars?{query}", headers={"Accept": "application/json"})
        with urlopen(request, timeout=self.timeout_seconds) as response:  # nosec B310: configured internal service URL
            payload = json.loads(response.read().decode("utf-8"))
        raw_bars = payload.get("bars")
        if not isinstance(raw_bars, list):
            raise ValueError("market core response is missing bars")
        return [Bar.from_market_core(row, instrument_id=instrument_id, timeframe=timeframe) for row in raw_bars]
