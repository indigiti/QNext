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

    if symbols:
        catalog = symbols["symbols"]
        nifty = next((s for s in catalog if isinstance(s, dict)
                      and "NIFTY" in str(s.get("ticker", "")).upper()
                      and "SYN" not in str(s.get("ticker", "")).upper()), None)
        if nifty and isinstance(nifty.get("instrument_id"), str):
            end = int(time.time() * 1000)
            start = end - 14 * 24 * 60 * 60 * 1000
            query = parse.urlencode({"instrument_id": nifty["instrument_id"],
                                     "timeframe": "1m", "from_ms": start,
                                     "to_ms": end, "limit": 100})
            check("historical NIFTY candles response", "/api/v1/bars/?" + query,
                  200, {"bars": list})
        else:
            results.append({"check": "historical NIFTY candles response", "result": "SKIP",
                            "reason": "symbol missing from public catalog"})

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
