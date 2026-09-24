"""QNext Strategy Lab deterministic replay and paper-execution core."""

from .engine import BacktestEngine, BacktestResult
from .execution import ExecutionConfig
from .models import Bar, Signal
from .strategies import MovingAverageCross

__all__ = [
    "BacktestEngine",
    "BacktestResult",
    "ExecutionConfig",
    "Bar",
    "Signal",
    "MovingAverageCross",
]
