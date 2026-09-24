from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Sequence

from .models import Bar, Signal


@dataclass(frozen=True)
class MovingAverageCross:
    fast: int = 2
    slow: int = 3
    name: str = "sma-cross"
    version: str = "1"

    def __post_init__(self) -> None:
        if self.fast < 1 or self.slow < 2 or self.fast >= self.slow:
            raise ValueError("require 1 <= fast < slow")

    @property
    def definition_hash(self) -> str:
        raw = json.dumps(
            {"name": self.name, "version": self.version, "fast": self.fast, "slow": self.slow},
            sort_keys=True,
            separators=(",", ":"),
        ).encode()
        return hashlib.sha256(raw).hexdigest()

    def on_bar(self, history: Sequence[Bar]) -> Signal | None:
        if len(history) < self.slow:
            return None
        fast_mean = sum(b.close for b in history[-self.fast :]) / self.fast
        slow_mean = sum(b.close for b in history[-self.slow :]) / self.slow
        direction = 1 if fast_mean > slow_mean else -1 if fast_mean < slow_mean else 0
        bar = history[-1]
        return Signal(
            signal_time_ms=bar.close_time_ms,
            target_direction=direction,
            reason=f"fast={fast_mean:.8f};slow={slow_mean:.8f}",
        )
