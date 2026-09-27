from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from .models import Bar
from .paper import PaperEngine, PaperSnapshot
from .strategies import MovingAverageCross


@dataclass(frozen=True)
class SynPlusPaperConfig:
    instrument_id: str = "QNEXT:NIFTY-SYN+"
    timeframe: str = "1m"
    initial_capital: float = 100_000.0
    unit_size: float = 1.0
    fast: int = 2
    slow: int = 3


class SynPlusPaperSession:
    """Paper-only adapter for the NIFTY-SYN+ shadow instrument.

    This adapter deliberately accepts only final quality-eligible bars for the
    configured SYN+ instrument/timeframe and delegates execution to the Q4
    PaperEngine. It has no broker client and no live-order escape hatch.
    """

    def __init__(self, config: SynPlusPaperConfig | None = None) -> None:
        self.config = config or SynPlusPaperConfig()
        strategy = MovingAverageCross(fast=self.config.fast, slow=self.config.slow)
        self.engine = PaperEngine(
            strategy,
            initial_capital=self.config.initial_capital,
            unit_size=self.config.unit_size,
        )

    def apply_bar(self, bar: Bar) -> PaperSnapshot:
        if bar.instrument_id != self.config.instrument_id:
            raise ValueError("SYN+ paper session received the wrong instrument")
        if bar.timeframe != self.config.timeframe:
            raise ValueError("SYN+ paper session received the wrong timeframe")
        return self.engine.apply(bar)

    def apply_payload(self, value: dict[str, Any]) -> PaperSnapshot:
        return self.apply_bar(Bar.from_dict(value))

    @property
    def broker_execution_enabled(self) -> bool:
        return False
