from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any


ALLOWED_QUALITY = {"GOOD", "RECOVERED"}


@dataclass(frozen=True)
class Bar:
    instrument_id: str
    timeframe: str
    open_time_ms: int
    close_time_ms: int
    open: float
    high: float
    low: float
    close: float
    volume: float = 0.0
    final: bool = True
    revision: int = 0
    quality: str = "GOOD"

    @classmethod
    def from_dict(cls, value: dict[str, Any]) -> "Bar":
        return cls(
            instrument_id=str(value["instrument_id"]),
            timeframe=str(value["timeframe"]),
            open_time_ms=int(value["open_time_ms"]),
            close_time_ms=int(value["close_time_ms"]),
            open=float(value["open"]),
            high=float(value["high"]),
            low=float(value["low"]),
            close=float(value["close"]),
            volume=float(value.get("volume", 0.0)),
            final=bool(value.get("final", True)),
            revision=int(value.get("revision", 0)),
            quality=str(value.get("quality", "GOOD")),
        )

    def canonical(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True)
class Signal:
    signal_time_ms: int
    target_direction: int
    reason: str = ""

    def __post_init__(self) -> None:
        if self.target_direction not in (-1, 0, 1):
            raise ValueError("target_direction must be -1, 0, or 1")


@dataclass(frozen=True)
class Order:
    order_id: str
    instrument_id: str
    created_at_ms: int
    quantity_delta: float
    signal_time_ms: int
    reason: str


@dataclass(frozen=True)
class Fill:
    fill_id: str
    order_id: str
    filled_at_ms: int
    quantity_delta: float
    reference_price: float
    price: float
    fees: float
    slippage_cost: float


@dataclass(frozen=True)
class EquityPoint:
    time_ms: int
    cash: float
    position_qty: float
    mark_price: float
    equity: float


@dataclass(frozen=True)
class ChartMarker:
    marker_id: str
    instrument_id: str
    time_ms: int
    action: str
    price: float
    quantity_delta: float
    label: str
