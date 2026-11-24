"""Create and reconstruct exact-base QNext release deltas.

This is an offline transport utility for verified, unpacked full releases.
It never modifies an installed release, runtime state, symlinks or services.
The receiver must obtain the target inventory digest and commit through an
independently approved release channel, not from the delta itself.
"""
import argparse
import hashlib
import json
import re
import shutil
import stat
import tempfile
import zipfile
from pathlib import Path, PurePosixPath

from release_inventory import NAME, inventory, read_inventory, compare, digest

META = "delta-manifest.json"
SCHEMA = "QNEXT.RELEASE.DELTA/1"
SHA = re.compile(r"[0-9a-f]{40}")
HASH = re.compile(r"[0-9a-f]{64}")
MAX_FILES = 20000
MAX_BYTES = 512 * 1024 * 1024


def require(condition, message):
    if not condition:
        raise ValueError(message)


def safe_path(value):
    p = PurePosixPath(value)
    require(value and not p.is_absolute() and p.as_posix() == value
            and all(x not in {".", ".."} for x in p.parts)
            and "\\" not in value and ":" not in value and "\x00" not in value,
            "unsafe release path")
    return p


def identity(root):
    actual = inventory(root)
    require(actual == read_inventory(root / NAME), "release inventory mismatch")
    require(not (root / NAME).is_symlink(), "inventory symlink")
    manifest = json.loads((root / "release-manifest.json").read_text())
    commit = manifest.get("commit", "")
    require(manifest.get("schema") == "QNEXT.RELEASE/1"
            and manifest.get("artifact") == "digiops-release"
            and SHA.fullmatch(commit), "invalid release identity")
    return commit, digest(root / NAME), actual


def create(base, target, output):
    bc, bh, before = identity(base)
    tc, th, after = identity(target)
    require(bc != tc, "base and target must differ")
    report = compare(before, after)
    changed = report["changed"]
    require(len(changed) <= MAX_FILES and report["changed_bytes"] <= MAX_BYTES,
            "delta exceeds transport limit")
    meta = {"schema": SCHEMA, "base_commit": bc, "base_inventory_sha256": bh,
            "target_commit": tc, "target_inventory_sha256": th,
            "changed": changed, "removed": report["removed"]}
    require(not output.exists(), "delta already exists")
    try:
        with zipfile.ZipFile(output, "x", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            z.writestr(META, json.dumps(meta, sort_keys=True))
            z.write(target / NAME, NAME)
            for path in changed:
                safe_path(path)
                z.write(target / path, "files/" + path)
    except BaseException:
        output.unlink(missing_ok=True)
        raise
    return report


def reconstruct(base, archive, destination, expected_base, expected_target, expected_inventory):
    require(SHA.fullmatch(expected_base) and SHA.fullmatch(expected_target)
            and HASH.fullmatch(expected_inventory), "approved SHA/digest required")
    require(not destination.exists() and not destination.is_symlink(), "destination already exists")
    bc, bh, before = identity(base)
    require(bc == expected_base, "wrong approved base")
    require(destination.parent.is_dir(), "destination parent must exist")
    temp = Path(tempfile.mkdtemp(prefix=".qnext-delta-", dir=destination.parent))
    try:
        with zipfile.ZipFile(archive) as z:
            entries = z.infolist()
            names = [x.filename for x in entries]
            require(len(entries) <= MAX_FILES + 2 and len(names) == len(set(names)), "invalid delta entries")
            require(sum(x.file_size for x in entries) <= MAX_BYTES, "delta exceeds transport limit")
            for entry in entries:
                safe_path(entry.filename)
                require(not entry.is_dir() and not stat.S_ISLNK(entry.external_attr >> 16),
                        "unexpected directory or symlink in delta")
            meta = json.loads(z.read(META))
            target_raw = z.read(NAME)
            require(hashlib.sha256(target_raw).hexdigest() == expected_inventory, "unapproved target inventory")
            require(meta.get("schema") == SCHEMA and meta.get("base_commit") == bc
                    and meta.get("base_inventory_sha256") == bh
                    and meta.get("target_commit") == expected_target
                    and meta.get("target_inventory_sha256") == expected_inventory, "delta identity mismatch")
            after = json.loads(target_raw)
            require(after.get("schema") == before["schema"], "unsupported target inventory")
            report = compare(before, after)
            changed, removed = report["changed"], report["removed"]
            require(meta.get("changed") == changed and meta.get("removed") == removed, "incorrect delta plan")
            require(set(names) == {META, NAME} | {"files/" + p for p in changed}, "unexpected delta payload")
            # Copy only files listed in the approved target. Removed files are
            # never copied, and no overlay can leave stale files behind.
            changed_paths = set(changed)
            for path, entry in after["files"].items():
                relative = safe_path(path)
                out = temp / relative
                out.parent.mkdir(parents=True, exist_ok=True)
                if path in changed_paths:
                    zi = z.getinfo("files/" + path)
                    require(zi.file_size == entry["bytes"], "changed file size mismatch")
                    with z.open(zi) as source, out.open("xb") as sink:
                        shutil.copyfileobj(source, sink)
                else:
                    shutil.copyfile(base / relative, out)
                require(out.stat().st_size == entry["bytes"] and digest(out) == entry["sha256"],
                        "reconstructed file digest mismatch: " + path)
            (temp / NAME).write_bytes(target_raw)
        tc, th, actual = identity(temp)
        require(tc == expected_target and th == expected_inventory and actual == after,
                "reconstructed release identity mismatch")
        # This is an unpacked compatibility ZIP payload, not a POSIX installer.
        # The existing staging installer restores its required executable modes.
        require(not destination.exists() and not destination.is_symlink(), "destination already exists")
        temp.rename(destination)
        return report
    finally:
        if temp.exists():
            shutil.rmtree(temp)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest="action", required=True)
    make = sub.add_parser("create")
    for name in ("base", "target", "output"):
        make.add_argument(name, type=Path)
    restore = sub.add_parser("reconstruct")
    for name in ("base", "archive", "destination"):
        restore.add_argument(name, type=Path)
    restore.add_argument("--base-commit", required=True)
    restore.add_argument("--target-commit", required=True)
    restore.add_argument("--target-inventory-sha256", required=True)
    a = p.parse_args()
    if a.action == "create":
        report = create(a.base, a.target, a.output)
    else:
        report = reconstruct(a.base, a.archive, a.destination, a.base_commit,
                             a.target_commit, a.target_inventory_sha256)
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
