from __future__ import annotations

import json
import os
import signal
import threading
import time
from dataclasses import asdict
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

from .models import Bar, Fill
from .syn_plus_paper import SynPlusPaperConfig, SynPlusPaperSession


RUNTIME_VERSION = "qnext-syn-plus-paper-runtime-v1"
DEFAULT_ADDR = "127.0.0.1:18082"
DEFAULT_INSTRUMENT = "QNEXT:NIFTY-SYN+"
ALLOWED_TIMEFRAMES = {
    "15s",
    "30s",
    "1m",
    "2m",
    "3m",
    "5m",
    "10m",
    "15m",
    "30m",
    "45m",
    "1h",
    "2h",
    "3h",
    "4h",
    "1D",
    "1W",
    "1M",
}


def _now_ms() -> int:
    return int(time.time() * 1000)


def _safe_component(value: str) -> str:
    return value.replace("/", "_").replace("\\", "_").replace(":", "_")


def _atomic_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temp = path.with_suffix(path.suffix + ".tmp")
    temp.write_text(json.dumps(payload, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    temp.replace(path)


def _date_from_ms(value: int) -> datetime:
    return datetime.fromtimestamp(value / 1000, tz=timezone.utc)


def _date_range(start_ms: int, end_ms: int) -> list[datetime]:
    start = _date_from_ms(start_ms).date()
    end = _date_from_ms(end_ms).date()
    days: list[datetime] = []
    current = start
    while current <= end:
        days.append(datetime(current.year, current.month, current.day, tzinfo=timezone.utc))
        current += timedelta(days=1)
    return days


def _jsonable_fill(fill: Fill) -> dict[str, Any]:
    return asdict(fill)


class SynPlusPaperRuntime:
    """Persistent forward-paper runtime for the SYN+ shadow instrument.

    The runtime never submits broker orders. Finalized SYN+ bars are copied into
    an append-only per-session event log before being applied to PaperEngine.
    Restarts rebuild paper state from that as-observed log, so later canonical
    history corrections cannot rewrite already-observed paper decisions.
    """

    def __init__(self, storage_root: str | Path) -> None:
        self.storage_root = Path(storage_root)
        self.root = self.storage_root / "paper" / "syn-plus"
        self.config_path = self.root / "runtime.json"
        self._lock = threading.RLock()
        self._history_offsets: dict[Path, int] = {}
        self._equity_curve: list[float] = []
        self._last_error = ""
        self._last_poll_ms = 0
        self._settings = self._load_settings()
        self._session: SynPlusPaperSession | None = None
        self._snapshot = None
        self._restore_session()

    @staticmethod
    def default_settings() -> dict[str, Any]:
        return {
            "schema": "QNEXT.SYN_PLUS_PAPER.RUNTIME/1",
            "enabled": False,
            "instrument_id": DEFAULT_INSTRUMENT,
            "timeframe": "1m",
            "initial_capital": 100_000.0,
            "unit_size": 1.0,
            "fast": 2,
            "slow": 3,
            "session_key": "",
            "started_at_ms": 0,
            "updated_at_ms": 0,
        }

    def _load_settings(self) -> dict[str, Any]:
        defaults = self.default_settings()
        if not self.config_path.is_file():
            return defaults
        try:
            raw = json.loads(self.config_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            return defaults
        if not isinstance(raw, dict):
            return defaults
        defaults.update(raw)
        return self._validate_settings(defaults, allow_enabled=True)

    def _validate_settings(
        self,
        raw: dict[str, Any],
        *,
        allow_enabled: bool,
    ) -> dict[str, Any]:
        instrument = str(raw.get("instrument_id", DEFAULT_INSTRUMENT)).strip()
        if instrument != DEFAULT_INSTRUMENT:
            raise ValueError("paper runtime is locked to QNEXT:NIFTY-SYN+")

        timeframe = str(raw.get("timeframe", "1m")).strip()
        if timeframe not in ALLOWED_TIMEFRAMES:
            raise ValueError("unsupported paper timeframe")

        initial_capital = float(raw.get("initial_capital", 100_000.0))
        unit_size = float(raw.get("unit_size", 1.0))
        fast = int(raw.get("fast", 2))
        slow = int(raw.get("slow", 3))
        if initial_capital <= 0:
            raise ValueError("initial_capital must be positive")
        if unit_size <= 0:
            raise ValueError("unit_size must be positive")
        if fast < 1 or slow < 2 or fast >= slow:
            raise ValueError("require 1 <= fast < slow")

        enabled = bool(raw.get("enabled", False)) if allow_enabled else False
        session_key = str(raw.get("session_key", "")).strip()
        started_at_ms = int(raw.get("started_at_ms", 0) or 0)
        updated_at_ms = int(raw.get("updated_at_ms", 0) or 0)

        return {
            "schema": "QNEXT.SYN_PLUS_PAPER.RUNTIME/1",
            "enabled": enabled,
            "instrument_id": instrument,
            "timeframe": timeframe,
            "initial_capital": initial_capital,
            "unit_size": unit_size,
            "fast": fast,
            "slow": slow,
            "session_key": session_key,
            "started_at_ms": started_at_ms,
            "updated_at_ms": updated_at_ms,
        }

    def _session_config(self) -> SynPlusPaperConfig:
        return SynPlusPaperConfig(
            instrument_id=self._settings["instrument_id"],
            timeframe=self._settings["timeframe"],
            initial_capital=self._settings["initial_capital"],
            unit_size=self._settings["unit_size"],
            fast=self._settings["fast"],
            slow=self._settings["slow"],
        )

    def _event_path(self) -> Path | None:
        key = self._settings.get("session_key", "")
        if not key:
            return None
        return self.root / "sessions" / key / "bars.jsonl"

    def _new_session_key(self, at_ms: int) -> str:
        dt = _date_from_ms(at_ms)
        return dt.strftime("%Y%m%dT%H%M%S") + f"-{at_ms % 1000:03d}"

    def _persist_settings(self) -> None:
        self._settings["updated_at_ms"] = _now_ms()
        _atomic_json(self.config_path, self._settings)

    def _restore_session(self) -> None:
        with self._lock:
            if not self._settings.get("session_key"):
                self._session = None
                self._snapshot = None
                self._equity_curve = []
                return

            self._session = SynPlusPaperSession(self._session_config())
            self._snapshot = None
            self._equity_curve = []
            event_path = self._event_path()
            if event_path is None or not event_path.is_file():
                return

            with event_path.open("r", encoding="utf-8") as handle:
                for line in handle:
                    line = line.strip()
                    if not line:
                        continue
                    payload = json.loads(line)
                    snapshot = self._session.apply_payload(payload)
                    self._snapshot = snapshot
                    self._equity_curve.append(snapshot.equity)

    def _start_new_session(self, *, enabled: bool, at_ms: int) -> None:
        self._settings["enabled"] = enabled
        self._settings["session_key"] = self._new_session_key(at_ms)
        self._settings["started_at_ms"] = at_ms
        self._persist_settings()
        self._history_offsets.clear()
        self._session = SynPlusPaperSession(self._session_config())
        self._snapshot = None
        self._equity_curve = []
        event_path = self._event_path()
        if event_path is not None:
            event_path.parent.mkdir(parents=True, exist_ok=True)

    def configure(self, payload: dict[str, Any]) -> dict[str, Any]:
        with self._lock:
            if self._settings.get("enabled"):
                raise ValueError("disable paper trading before changing configuration")
            candidate = dict(self._settings)
            for key in ("timeframe", "initial_capital", "unit_size", "fast", "slow"):
                if key in payload:
                    candidate[key] = payload[key]
            candidate = self._validate_settings(candidate, allow_enabled=True)
            candidate["session_key"] = self._settings.get("session_key", "")
            candidate["started_at_ms"] = self._settings.get("started_at_ms", 0)
            self._settings = candidate
            self._persist_settings()
            self._restore_session()
            return self.status(poll=False)

    def control(self, payload: dict[str, Any]) -> dict[str, Any]:
        action = str(payload.get("action", "")).strip().lower()
        now_ms = int(payload.get("at_ms", 0) or _now_ms())
        with self._lock:
            if action == "enable":
                if self._settings.get("enabled"):
                    return self.status(poll=False)
                candidate = dict(self._settings)
                for key in ("timeframe", "initial_capital", "unit_size", "fast", "slow"):
                    if key in payload:
                        candidate[key] = payload[key]
                self._settings = self._validate_settings(candidate, allow_enabled=True)
                self._start_new_session(enabled=True, at_ms=now_ms)
            elif action == "disable":
                self._settings["enabled"] = False
                self._persist_settings()
            elif action == "reset":
                self._start_new_session(enabled=bool(self._settings.get("enabled")), at_ms=now_ms)
            elif action == "configure":
                return self.configure(payload)
            else:
                raise ValueError("action must be enable, disable, reset, or configure")
        return self.status(poll=False)

    def _history_path(self, day: datetime) -> Path:
        return (
            self.storage_root
            / "market"
            / _safe_component(self._settings["instrument_id"])
            / self._settings["timeframe"]
            / day.strftime("%Y")
            / day.strftime("%m")
            / (day.strftime("%Y-%m-%d") + ".jsonl")
        )

    def _read_history_delta(self, path: Path) -> list[dict[str, Any]]:
        if not path.is_file():
            return []
        size = path.stat().st_size
        offset = self._history_offsets.get(path, 0)
        if offset > size:
            offset = 0
        records: list[dict[str, Any]] = []
        with path.open("r", encoding="utf-8") as handle:
            handle.seek(offset)
            while True:
                before = handle.tell()
                line = handle.readline()
                if line == "":
                    break
                if not line.endswith("\n"):
                    handle.seek(before)
                    break
                try:
                    decoded = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if isinstance(decoded, dict):
                    records.append(decoded)
            self._history_offsets[path] = handle.tell()
        return records

    def _append_event(self, payload: dict[str, Any]) -> None:
        event_path = self._event_path()
        if event_path is None:
            raise RuntimeError("paper session has no event path")
        event_path.parent.mkdir(parents=True, exist_ok=True)
        encoded = json.dumps(payload, sort_keys=True, separators=(",", ":")) + "\n"
        with event_path.open("a", encoding="utf-8") as handle:
            handle.write(encoded)
            handle.flush()
            os.fsync(handle.fileno())

    def _last_open_time_ms(self) -> int:
        if self._session is None or not self._session.engine.history:
            return -1
        return self._session.engine.history[-1].open_time_ms

    def _eligible_payload(self, payload: dict[str, Any]) -> bool:
        if str(payload.get("instrument_id", "")) != self._settings["instrument_id"]:
            return False
        if str(payload.get("timeframe", "")) != self._settings["timeframe"]:
            return False
        if not bool(payload.get("final", False)):
            return False
        if str(payload.get("quality", "")) not in {"GOOD", "RECOVERED"}:
            return False
        try:
            close_time_ms = int(payload.get("close_time_ms", 0))
            open_time_ms = int(payload.get("open_time_ms", 0))
        except (TypeError, ValueError):
            return False
        if close_time_ms < int(self._settings.get("started_at_ms", 0)):
            return False
        if open_time_ms <= self._last_open_time_ms():
            return False
        return True

    def poll_once(self, *, now_ms: int | None = None) -> int:
        with self._lock:
            self._last_poll_ms = now_ms or _now_ms()
            if not self._settings.get("enabled"):
                return 0
            if self._session is None:
                self._restore_session()
            if self._session is None:
                return 0

            started = int(self._settings.get("started_at_ms", self._last_poll_ms))
            records: list[dict[str, Any]] = []
            for day in _date_range(started, self._last_poll_ms):
                records.extend(self._read_history_delta(self._history_path(day)))
            records.sort(key=lambda item: int(item.get("open_time_ms", 0) or 0))

            applied = 0
            for payload in records:
                if not self._eligible_payload(payload):
                    continue
                try:
                    Bar.from_dict(payload)
                    self._append_event(payload)
                    self._snapshot = self._session.apply_payload(payload)
                    self._equity_curve.append(self._snapshot.equity)
                    self._last_error = ""
                    applied += 1
                except (KeyError, TypeError, ValueError, OSError, json.JSONDecodeError) as error:
                    self._last_error = str(error)
            return applied

    def _trade_metrics(self) -> dict[str, Any]:
        session = self._session
        snapshot = self._snapshot
        if session is None or snapshot is None:
            return {
                "realized_pnl": 0.0,
                "unrealized_pnl": 0.0,
                "total_pnl": 0.0,
                "avg_entry_price": 0.0,
                "trade_count": 0,
                "win_count": 0,
                "win_rate": 0.0,
                "max_drawdown": 0.0,
                "max_drawdown_pct": 0.0,
            }

        position = 0.0
        avg_price = 0.0
        realized = 0.0
        closed_results: list[float] = []
        eps = 1e-12

        for fill in session.engine.fills:
            delta = fill.quantity_delta
            if abs(position) < eps or position * delta > 0:
                new_position = position + delta
                if abs(position) < eps:
                    avg_price = fill.price
                else:
                    avg_price = (
                        abs(position) * avg_price + abs(delta) * fill.price
                    ) / abs(new_position)
                position = new_position
                realized -= fill.fees
                continue

            closing = min(abs(position), abs(delta))
            close_pnl = closing * (fill.price - avg_price) * (1.0 if position > 0 else -1.0)
            close_pnl -= fill.fees
            realized += close_pnl
            closed_results.append(close_pnl)
            new_position = position + delta
            if abs(new_position) < eps:
                position = 0.0
                avg_price = 0.0
            elif position * new_position > 0:
                position = new_position
            else:
                position = new_position
                avg_price = fill.price

        unrealized = position * (snapshot.mark_price - avg_price)
        total = snapshot.equity - session.config.initial_capital
        peak = session.config.initial_capital
        max_drawdown = 0.0
        max_drawdown_pct = 0.0
        for equity in self._equity_curve:
            peak = max(peak, equity)
            drawdown = max(0.0, peak - equity)
            max_drawdown = max(max_drawdown, drawdown)
            if peak > 0:
                max_drawdown_pct = max(max_drawdown_pct, drawdown / peak)
        wins = sum(1 for value in closed_results if value > 0)
        trade_count = len(closed_results)

        return {
            "realized_pnl": realized,
            "unrealized_pnl": unrealized,
            "total_pnl": total,
            "avg_entry_price": avg_price,
            "trade_count": trade_count,
            "win_count": wins,
            "win_rate": wins / trade_count if trade_count else 0.0,
            "max_drawdown": max_drawdown,
            "max_drawdown_pct": max_drawdown_pct,
        }

    def status(self, *, poll: bool = True) -> dict[str, Any]:
        if poll:
            try:
                self.poll_once()
            except Exception as error:  # runtime boundary; preserve status visibility
                self._last_error = str(error)

        with self._lock:
            snapshot = self._snapshot
            session = self._session
            metrics = self._trade_metrics()
            last_bar = None
            if session is not None and session.engine.history:
                last_bar = session.engine.history[-1].canonical()

            fills = []
            markers = []
            pending = None
            cash = self._settings["initial_capital"]
            position_qty = 0.0
            mark_price = 0.0
            equity = self._settings["initial_capital"]
            paper_session_id = ""
            if snapshot is not None:
                fills = [_jsonable_fill(fill) for fill in snapshot.fills[-50:]]
                markers = [marker.to_dict() for marker in snapshot.markers[-100:]]
                pending = asdict(snapshot.pending_order) if snapshot.pending_order else None
                cash = snapshot.cash
                position_qty = snapshot.position_qty
                mark_price = snapshot.mark_price
                equity = snapshot.equity
                paper_session_id = snapshot.session_id

            return {
                "schema": "QNEXT.SYN_PLUS_PAPER.STATUS/1",
                "runtime_version": RUNTIME_VERSION,
                "enabled": bool(self._settings.get("enabled")),
                "broker_execution_enabled": False,
                "instrument_id": self._settings["instrument_id"],
                "timeframe": self._settings["timeframe"],
                "strategy": {
                    "name": "sma-cross",
                    "fast": self._settings["fast"],
                    "slow": self._settings["slow"],
                },
                "initial_capital": self._settings["initial_capital"],
                "unit_size": self._settings["unit_size"],
                "session_key": self._settings.get("session_key", ""),
                "paper_session_id": paper_session_id,
                "started_at_ms": self._settings.get("started_at_ms", 0),
                "last_poll_ms": self._last_poll_ms,
                "last_error": self._last_error,
                "cash": cash,
                "position_qty": position_qty,
                "mark_price": mark_price,
                "equity": equity,
                "pending_order": pending,
                "last_bar": last_bar,
                "metrics": metrics,
                "fills": fills,
                "markers": markers,
            }


class _Handler(BaseHTTPRequestHandler):
    runtime: SynPlusPaperRuntime

    def _send(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _body(self) -> dict[str, Any]:
        length = int(self.headers.get("Content-Length", "0") or 0)
        if length <= 0:
            return {}
        raw = self.rfile.read(length)
        decoded = json.loads(raw.decode("utf-8"))
        if not isinstance(decoded, dict):
            raise ValueError("request body must be an object")
        return decoded

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/health":
            self._send(200, {"ok": True, "version": RUNTIME_VERSION})
            return
        if self.path == "/status":
            self._send(200, self.runtime.status())
            return
        self._send(404, {"error": "not found"})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/control":
            self._send(404, {"error": "not found"})
            return
        try:
            self._send(200, self.runtime.control(self._body()))
        except (ValueError, json.JSONDecodeError) as error:
            self._send(400, {"error": str(error)})

    def log_message(self, format: str, *args: Any) -> None:
        return


def _split_addr(value: str) -> tuple[str, int]:
    host, sep, port_raw = value.rpartition(":")
    if not sep or not host:
        raise ValueError("paper runtime address must be host:port")
    port = int(port_raw)
    if port < 1 or port > 65535:
        raise ValueError("paper runtime port is invalid")
    return host, port


def main() -> None:
    storage_root = os.environ.get("QNEXT_STORAGE_ROOT", "./storage")
    addr = os.environ.get("QNEXT_SYN_PLUS_PAPER_ADDR", DEFAULT_ADDR)
    runtime = SynPlusPaperRuntime(storage_root)
    host, port = _split_addr(addr)

    handler = type("SynPlusPaperHandler", (_Handler,), {"runtime": runtime})
    server = ThreadingHTTPServer((host, port), handler)
    stop_event = threading.Event()

    def poll_loop() -> None:
        while not stop_event.wait(1.0):
            try:
                runtime.poll_once()
            except Exception as error:  # process stays observable on bad input/filesystem
                runtime._last_error = str(error)

    thread = threading.Thread(target=poll_loop, name="syn-plus-paper-poller", daemon=True)
    thread.start()

    def request_stop(_signum: int, _frame: Any) -> None:
        threading.Thread(target=server.shutdown, daemon=True).start()

    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)

    try:
        server.serve_forever(poll_interval=0.5)
    finally:
        stop_event.set()
        server.server_close()
        thread.join(timeout=2.0)


if __name__ == "__main__":
    main()
