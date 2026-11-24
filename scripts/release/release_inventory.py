"""Deterministic file inventory for complete QNext ZIP releases.

The inventory is informational for old installers; it does not authorize
partial installation or replace the approved release commit/digest.
"""
import argparse
import hashlib
import json
from pathlib import Path

NAME = "release-inventory.json"
SCHEMA = "QNEXT.RELEASE.INVENTORY/1"


def component(path):
    if path == "release-manifest.json" or path == "public/qnext-release.json":
        return "release-metadata"
    if path.startswith("public/admin/"):
        return "admin"
    if path.startswith("public/"):
        return "workspace"
    if path.startswith("private/bin/qnext-market-core"):
        return "market-core"
    if path.startswith("private/bin/qnext-data-collector"):
        return "data-collector"
    for prefix, name in (
        ("private/strategy-lab/", "strategy-lab"),
        ("private/intelligence/", "intelligence"),
        ("private/ops-api/", "ops-api"),
        ("private/config/", "config-templates"),
        ("private/deploy/", "deployment-tools"),
    ):
        if path.startswith(prefix):
            return name
    raise ValueError(f"Unclassified release path: {path}")


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def inventory(root):
    files = {}
    components = {}
    for file in sorted(root.rglob("*")):
        path = file.relative_to(root).as_posix()
        if file.is_symlink():
            raise ValueError(f"Unexpected ZIP symlink: {path}")
        if file.is_dir() or path == NAME:
            continue
        if not file.is_file():
            raise ValueError(f"Unexpected file type: {path}")
        if "__pycache__" in file.parts or file.suffix in {".pyc", ".pyo", ".map"} or path.startswith("private/ops-api/tests/"):
            raise ValueError(f"Non-runtime payload: {path}")
        group = component(path)
        entry = {"component": group, "bytes": file.stat().st_size, "sha256": digest(file)}
        files[path] = entry
        summary = components.setdefault(group, {"bytes": 0, "files": 0})
        summary["bytes"] += entry["bytes"]
        summary["files"] += 1
    if not files:
        raise ValueError("Empty release")
    return {"schema": SCHEMA, "files": files, "components": components}


def read_inventory(path):
    result = json.loads(path.read_text(encoding="utf-8"))
    if result.get("schema") != SCHEMA:
        raise ValueError("Unsupported inventory schema")
    return result


def compare(before, after):
    old, new = before["files"], after["files"]
    changed = sorted(p for p in new if old.get(p) != new[p])
    removed = sorted(set(old) - set(new))
    return {
        "changed": changed,
        "removed": removed,
        "changed_components": sorted({new[p]["component"] for p in changed} | {old[p]["component"] for p in removed}),
        "changed_bytes": sum(new[p]["bytes"] for p in changed),
        "full_bytes": sum(e["bytes"] for e in new.values()),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["create", "verify", "compare"])
    parser.add_argument("path", type=Path)
    parser.add_argument("other", type=Path, nargs="?")
    args = parser.parse_args()
    if args.action == "compare":
        if not args.other:
            parser.error("compare requires two inventory files")
        print(json.dumps(compare(read_inventory(args.path), read_inventory(args.other)), indent=2))
        return
    actual = inventory(args.path)
    manifest = args.path / NAME
    if args.action == "create":
        manifest.write_text(json.dumps(actual, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    elif read_inventory(manifest) != actual:
        raise SystemExit("Release inventory mismatch")
    print(json.dumps(actual["components"], sort_keys=True))
    print("Total runtime bytes:", sum(e["bytes"] for e in actual["components"].values()))


if __name__ == "__main__":
    main()
