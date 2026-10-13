#!/usr/bin/env bash
# Fast fake-wheel tests of the package installer and fail-safe Lab integration.
set -euo pipefail
cd "$(dirname "$0")/../.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
root="$tmp/qnext"
source_dir="$tmp/wheels"
mkdir -p "$root/releases/test/private/config" "$source_dir"
cp config/intelligence-lab-ml.requirements.lock "$root/releases/test/private/config/"
ln -s "$root/releases/test" "$root/current"
cp config/intelligence-lab-ml.requirements.lock "$source_dir/requirements.lock"
printf 'synthetic test bytes\n' > "$source_dir/test_package-1.0-py3-none-any.whl"
(
  cd "$source_dir"
  sha256sum requirements.lock *.whl > wheelhouse.sha256
)
export QNEXT_PRIVATE_ROOT="$root"
bash deploy/qnext-ml-dependencies verify "$source_dir"
bash deploy/qnext-ml-dependencies install "$source_dir"
bash deploy/qnext-ml-dependencies status
first="$(readlink "$root/ml-dependencies/current")"
bash deploy/qnext-ml-dependencies install "$source_dir"
test "$(readlink "$root/ml-dependencies/current")" = "$first"
if bash deploy/qnext-intelligence-lab-python status >/dev/null; then
  echo "Lab reports READY without an installed environment" >&2
  exit 1
fi
grep -Eq '"state":"(NEEDS_BOOTSTRAP|UNAVAILABLE)"' "$root/run/intelligence-lab-python.json"

printf 'tampered bytes\n' >> "$source_dir/test_package-1.0-py3-none-any.whl"
if bash deploy/qnext-ml-dependencies install "$source_dir" >/dev/null 2>&1; then
  echo "corrupted wheelhouse was accepted" >&2
  exit 1
fi
test "$(readlink "$root/ml-dependencies/current")" = "$first"

printf 'new package bytes\n' > "$source_dir/test_package-1.0-py3-none-any.whl"
(
  cd "$source_dir"
  sha256sum requirements.lock *.whl > wheelhouse.sha256
)
bash deploy/qnext-ml-dependencies install "$source_dir"
second="$(readlink "$root/ml-dependencies/current")"
test "$second" != "$first"
test -d "$root/ml-dependencies/$first"
echo '# test lock change' >> "$root/releases/test/private/config/intelligence-lab-ml.requirements.lock"
if bash deploy/qnext-ml-dependencies status >/dev/null 2>&1; then
  echo "mismatched QNext release lock was accepted" >&2
  exit 1
fi
if bash deploy/qnext-intelligence-lab-python status >/dev/null; then
  echo "mismatched dependency lock was treated as READY" >&2
  exit 1
fi
grep -q '"state":"UNAVAILABLE"' "$root/run/intelligence-lab-python.json"
echo "QNext split ML package contract PASS"
