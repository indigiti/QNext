from __future__ import annotations

import hashlib
from dataclasses import asdict, dataclass
from typing import Protocol

from .execution import DeterministicMarketExecution, ExecutionConfig
from .models import ALLOWED_QUALITY, Bar, Fill, Order, Signal


class Strategy(Protocol):
    name: str
    version: str
    definition_hash: str

    def on_bar(self, history: list[Bar]) -> Signal | None: ...


@dataclass(frozen=True)
class ChartMarker:
    marker_id: str
    time_ms: int
    kind: str
    side: str
    price: float
    label: str
    order_id: str = ""
    fill_id: str = ""

    def to_dict(self) -> dict:
        return asdict(self)


@dataclass(frozen=True)
class PaperSnapshot:
    session_id: str
    instrument_id: str
    timeframe: str
    as_of_time_ms: int
    cash: float
    position_qty: float
    mark_price: float
    equity: float
    pending_order: Order | None
    fills: tuple[Fill, ...]
    markers: tuple[ChartMarker, ...]

    def to_dict(self) -> dict:
        value = asdict(self)
        value["pending_order"] = asdict(self.pending_order) if self.pending_order else None
        value["fills"] = [asdict(fill) for fill in self.fills]
        value["markers"] = [marker.to_dict() for marker in self.markers]
        return value


class PaperEngine:
    VERSION = "qnext-paper-v1"

    def __init__(
        self,
        strategy: Strategy,
        *,
        initial_capital: float = 100_000.0,
        unit_size: float = 1.0,
        execution_config: ExecutionConfig | None = None,
    ) -> None:
        if initial_capital <= 0 or unit_size <= 0:
            raise ValueError("initial_capital and unit_size must be positive")
        self.strategy = strategy
        self.initial_capital = float(initial_capital)
        self.unit_size = float(unit_size)
        self.execution = DeterministicMarketExecution(execution_config)
        self.cash = self.initial_capital
        self.position_qty = 0.0
        self.pending: Order | None = None
        self.history: list[Bar] = []
        self.fills: list[Fill] = []
        self.markers: list[ChartMarker] = []
        self.order_seq = 0
        self.fill_seq = 0
        self._instrument_id = ""
        self._timeframe = ""
        self._session_id = ""

    def apply(self, bar: Bar) -> PaperSnapshot:
        self._validate_bar(bar)

        if not self._session_id:
            seed = "|".join(
                [
                    self.VERSION,
                    bar.instrument_id,
                    bar.timeframe,
                    self.strategy.definition_hash,
                    self.execution.VERSION,
                    f"{self.initial_capital:.8f}",
                    f"{self.unit_size:.8f}",
                ]
            )
            self._session_id = hashlib.sha256(seed.encode()).hexdigest()[:24]
            self._instrument_id = bar.instrument_id
            self._timeframe = bar.timeframe

        if self.pending is not None:
            self.fill_seq += 1
            fill = self.execution.fill(
                self.pending,
                bar,
                f"{self._session_id}-F{self.fill_seq:06d}",
            )
            self.cash -= fill.quantity_delta * fill.price
            self.cash -= fill.fees
            self.position_qty += fill.quantity_delta
            self.fills.append(fill)
            side = "BUY" if fill.quantity_delta > 0 else "SELL"
            self.markers.append(
                ChartMarker(
                    marker_id=f"{fill.fill_id}-MARK",
                    time_ms=fill.filled_at_ms,
                    kind="FILL",
                    side=side,
                    price=fill.price,
                    label=f"{side} fill",
                    order_id=fill.order_id,
                    fill_id=fill.fill_id,
                )
            )
            self.pending = None

        self.history.append(bar)
        signal = self.strategy.on_bar(self.history)
        if signal is not None:
            if signal.signal_time_ms != bar.close_time_ms:
                raise ValueError("strategy signal time must equal current final bar close")
            target_qty = signal.target_direction * self.unit_size
            quantity_delta = target_qty - self.position_qty
            if abs(quantity_delta) >= 1e-12:
                self.order_seq += 1
                self.pending = Order(
                    order_id=f"{self._session_id}-O{self.order_seq:06d}",
                    instrument_id=bar.instrument_id,
                    created_at_ms=bar.close_time_ms,
                    quantity_delta=quantity_delta,
                    signal_time_ms=signal.signal_time_ms,
                    reason=signal.reason,
                )
                side = "BUY" if quantity_delta > 0 else "SELL"
                self.markers.append(
                    ChartMarker(
                        marker_id=f"{self.pending.order_id}-SIGNAL",
                        time_ms=bar.close_time_ms,
                        kind="SIGNAL",
                        side=side,
                        price=bar.close,
                        label=signal.reason or f"{side} signal",
                        order_id=self.pending.order_id,
                    )
                )

        equity = self.cash + self.position_qty * bar.close
        return PaperSnapshot(
            session_id=self._session_id,
            instrument_id=bar.instrument_id,
            timeframe=bar.timeframe,
            as_of_time_ms=bar.close_time_ms,
            cash=self.cash,
            position_qty=self.position_qty,
            mark_price=bar.close,
            equity=equity,
            pending_order=self.pending,
            fills=tuple(self.fills),
            markers=tuple(self.markers),
        )

    def _validate_bar(self, bar: Bar) -> None:
        if not bar.final:
            raise ValueError("paper engine requires final bars")
        if bar.quality not in ALLOWED_QUALITY:
            raise ValueError(f"bar quality not eligible for Q4 paper engine: {bar.quality}")
        if bar.close_time_ms <= bar.open_time_ms:
            raise ValueError("bar close time must be after open time")
        if not (bar.low <= min(bar.open, bar.close) <= max(bar.open, bar.close) <= bar.high):
            raise ValueError("invalid OHLC envelope")
        if self.history:
            previous = self.history[-1]
            if bar.instrument_id != previous.instrument_id or bar.timeframe != previous.timeframe:
                raise ValueError("paper session must use one instrument and timeframe")
            if bar.open_time_ms <= previous.open_time_ms:
                raise ValueError("paper bars must be strictly ordered with no duplicates")
