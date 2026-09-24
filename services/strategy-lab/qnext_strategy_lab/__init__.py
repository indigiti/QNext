"""QNext Strategy Lab deterministic replay and paper-execution core."""

from .engine import BacktestEngine, BacktestResult
from .execution import ExecutionConfig
from .models import Bar, ChartMarker, Signal
from .paper import PaperEngine, PaperResult
from .strategies import MovingAverageCross

__all__ = [
    "BacktestEngine",
    "BacktestResult",
    "ExecutionConfig",
    "Bar",
    "ChartMarker",
    "Signal",
    "PaperEngine",
    "PaperResult",
    "MovingAverageCross",
]
