#!/usr/bin/env python3
"""Read-only QNeXT staging HTTP and access-boundary probe; no credentials or mutations."""
from __future__ import annotations

import argparse
import json
import sys
import time
from urllib import error, parse, request


def fetch(base: str, path: str, timeout: float) -> tuple[int, bytes, str]:
    target = base.rstrip("/") + "/" + path.lstrip("/")
    req = request.Request(target, method="GET", headers={"Accept": "application/json, text/html"})
    try:
        with request.urlopen(req, timeout=timeout) as response:
            return response.status, response.read(2_000_000), response.headers.get("Content-Type", "")
    except error.HTTPError as exc:
        return exc.code, exc.read(2_000_000), exc.headers.get("Content-Type", "")


def validate_json(body: bytes, required: dict[str, type]) -> dict:
    doc = json.loads(body)
    if not isinstance(doc, dict):
        raise ValueError("JSON object expected")
    for key, expected in required.items():
        if key not in doc or not isinstance(doc[key], expected):
            raise ValueError(f"missing or invalid {key}")
    return doc


def validate_bars(bars: list, label: str) -> None:
    """Validate the raw API contract; do not silently normalize missing data."""
    import math
    if not bars:
        raise ValueError(f"{label}: no historical candles returned")
    previous = -1
    volume_seen = False
    for index, bar in enumerate(bars):
        if not isinstance(bar, dict):
            raise ValueError(f"{label}: candle {index} is not an object")
        fields = {}
        for key in ("time", "open", "high", "low", "close"):
            value = bar.get(key)
            if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
                raise ValueError(f"{label}: invalid {key} in candle {index}")
            fields[key] = value
        if fields["time"] <= previous:
            raise ValueError(f"{label}: duplicate or non-increasing candle time at {index}")
        previous = fields["time"]
        if fields["high"] < max(fields["open"], fields["close"], fields["low"]) or (
            fields["low"] > min(fields["open"], fields["close"], fields["high"])
        ):
            raise ValueError(f"{label}: OHLC invariant violated at {index}")
        volume = bar.get("volume")
        if volume is not None:
            if isinstance(volume, bool) or not isinstance(volume, (int, float)) or not math.isfinite(volume) or volume < 0:
                raise ValueError(f"{label}: invalid volume in candle {index}")
            volume_seen = True
    if not volume_seen:
        raise ValueError(f"{label}: all historical candle volumes unavailable")


def classify_nifty(ticker: str) -> str | None:
    normalized = ticker.upper().split(":")[-1].replace(" ", "").replace("_", "")
    if not normalized.startswith("NIFTY") or any(
        prefix in normalized for prefix in ("BANK", "FIN", "MIDCAP", "NEXT", "IT")
    ):
        return None
    if "SYN+" in normalized or "SYNPLUS" in normalized:
        return "NIFTY SYN+"
    if "SYN" in normalized:
        return "NIFTY SYN"
    if normalized in ("NIFTY", "NIFTY50", "NIFTY-50"):
        return "NIFTY"
    return None


def inspect(base: str, timeout: float, public_markers: bool) -> list[dict]:
    results: list[dict] = []

    def check(name: str, path: str, expected: int, required: dict[str, type] | None = None) -> dict | None:
        try:
            status, body, content_type = fetch(base, path, timeout)
            if status != expected:
                raise ValueError(f"HTTP {status}, expected {expected}")
            if required is not None:
                document = validate_json(body, required)
            else:
                if "html" not in content_type.lower() and b"<html" not in body.lower():
                    raise ValueError("HTML page not returned")
                document = {}
            results.append({"check": name, "result": "PASS", "status": status})
            return document
        except Exception as exc:
            results.append({"check": name, "result": "FAIL", "error": str(exc)})
            return None

    check("workspace page", "/", 200)
    check("admin entry page", "/admin/", 200)
    check("public timeframes", "/api/v1/timeframes/", 200, {"timeframes": list})
    symbols = check("public symbol catalog", "/api/v1/symbols/", 200, {"symbols": list})
    check("workspace settings", "/api/v1/workspace-settings/", 200, {"chartEngine": str})
    check("feed status", "/api/v1/feed-status/", 200, {})
    paper = check("paper marker visibility", "/api/v1/paper-markers/", 200,
                  {"available": bool, "enabled": bool, "markers": list})
    if paper is not None and not public_markers:
        if paper["markers"] or paper["available"] or paper["enabled"]:
            results.append({"check": "no public paper executions", "result": "FAIL",
                            "error": "public paper execution data is enabled"})
        else:
            results.append({"check": "no public paper executions", "result": "PASS"})

    def admin_route(route: str) -> str:
        return "/admin/api/index.php?" + parse.urlencode({"route": route})
    check("admin session metadata", admin_route("/session"), 200, {"authenticated": bool})
    check("admin configuration forbidden", admin_route("/config"), 403, {"error": str})
    check("admin status forbidden", admin_route("/status"), 403, {"error": str})
    check("admin diagnostics forbidden", admin_route("/diagnostics"), 403, {"error": str})

    targets = {"NIFTY": None, "NIFTY SYN": None, "NIFTY SYN+": None}
    if symbols:
        for entry in symbols["symbols"]:
            if not isinstance(entry, dict):
                continue
            category = classify_nifty(str(entry.get("ticker", "")))
            if category and targets[category] is None:
                targets[category] = entry.get("instrument_id")

    end_ms = int(time.time() * 1000)
    start_ms = end_ms - 14 * 24 * 60 * 60 * 1000
    for label, instrument_id in targets.items():
        name = f"historical {label} candles and volume"
        if not isinstance(instrument_id, str) or not instrument_id:
            results.append({"check": name, "result": "FAIL",
                            "error": "symbol is missing from public catalog"})
            continue
        query = parse.urlencode({"instrument_id": instrument_id,
                                 "timeframe": "1m", "from_ms": start_ms,
                                 "to_ms": end_ms, "limit": 100})
        try:
            status, body, _ = fetch(base, "/api/v1/bars/?" + query, timeout)
            if status != 200:
                raise ValueError(f"HTTP {status}, expected 200")
            payload = validate_json(body, {"bars": list})
            validate_bars(payload["bars"], label)
            results.append({"check": name, "result": "PASS",
                            "bars_checked": len(payload["bars"])})
        except Exception as exc:
            results.append({"check": name, "result": "FAIL", "error": str(exc)})

    return results


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="https://stage.digiti.in/qnext/")
    parser.add_argument("--timeout", type=float, default=6.0)
    parser.add_argument("--allow-public-markers", action="store_true",
                        help="accept explicitly enabled public paper execution events")
    args = parser.parse_args()
    checks = inspect(args.url, args.timeout, args.allow_public_markers)
    for item in checks:
        print(f"{item['result']:4} {item['check']} "
              f"{item.get('error', item.get('reason', ''))}")
    failed = sum(c["result"] == "FAIL" for c in checks)
    print(f"Staging read-only smoke: {len(checks) - failed} non-fail, {failed} failed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
