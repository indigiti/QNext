#!/usr/bin/env bash
set -euo pipefail
root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT
repo_root="$(cd "$(dirname "$0")/../.." && pwd -P)"
payload="$root/artifact"
private="$root/private"
version="0123456789ab"
sha="0123456789abcdef0123456789abcdef01234567"
mkdir -p "$payload/public/admin/api" "$payload/public/api/v1/bars" \
  "$payload/private/bin" "$payload/private/ops-api/public" \
  "$payload/private/deploy" "$private/storage" "$private/logs"
for needed in public/index.html public/admin/index.html \
  public/admin/api/index.php public/api/v1/bars/index.php \
  public/qnext-release.json private/ops-api/public/index.php \
  private/deploy/qnext-ops-user; do
  printf 'fixture\n' > "$payload/$needed"
done
cat > "$payload/release-manifest.json" <<'JSON'
{"schema":"QNEXT.RELEASE/1","artifact":"digiops-release","commit":"0123456789abcdef0123456789abcdef01234567","version":"0123456789ab"}
JSON
for binary in qnext-market-core qnext-data-collector; do
  cp /bin/true "$payload/private/bin/$binary-$version"
  printf '%s\n' "$binary-$version" > "$payload/private/bin/$binary.current"
  ln -s "$binary-$version" "$payload/private/bin/$binary"
done

bash "$repo_root/deploy/qnext-stage-release" "$payload" "$private" "$sha" >/dev/null
test -f "$private/releases/$version/release-manifest.json"
test -L "$private/releases/$version/private/bin/qnext-market-core"
test ! -e "$private/current"
test -d "$private/storage"
test -d "$private/logs"

# Staged releases are immutable and must not be replaced after upload.
if bash "$repo_root/deploy/qnext-stage-release" "$payload" "$private" "$sha" >/dev/null 2>&1; then
  echo 'FAILED: staging an existing release was allowed' >&2
  exit 1
fi

if bash "$repo_root/deploy/qnext-stage-release" "$payload" "$private" "0000000000000000000000000000000000000000" >/dev/null 2>&1; then
  echo 'FAILED: manifest commit mismatch was allowed' >&2
  exit 1
fi

rm "$payload/private/bin/qnext-market-core"
ln -s /bin/true "$payload/private/bin/qnext-market-core"
mv "$private/releases/$version" "$private/releases/previous-fixture"
if bash "$repo_root/deploy/qnext-stage-release" "$payload" "$private" "$sha" >/dev/null 2>&1; then
  echo 'FAILED: absolute binary symlink was allowed' >&2
  exit 1
fi
test ! -e "$private/releases/$version"
echo 'QNext immutable staging release installer: PASS'
