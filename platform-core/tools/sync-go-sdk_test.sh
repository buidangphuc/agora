#!/usr/bin/env bash
# Test for tools/sync-go-sdk.sh. Runs against a throwaway clone of this repo
# (so tags are created there, never in the real repo) and a scratch copy of
# testdata/sync-fixture, a minimal service that no SDK change can break. Run: bash platform-core/tools/sync-go-sdk_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
SYNC="$SCRIPT_DIR/sync-go-sdk.sh"
TAG="go-sdk/v0.0.0-test"
MISSING_TAG="go-sdk/v9.9.9-missing"
SDK_MOD="github.com/buidangphuc/platform-core/packages/go-sdk"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/sync-go-sdk-test.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok: $*"; }

# Fingerprint of a tree: file names plus content checksums.
tree_sum() {
  (cd "$1" && find . | LC_ALL=C sort && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 cksum) | cksum
}

[ -f "$SYNC" ] || fail "script missing: $SYNC"

# 1. Throwaway clone with a local tag. The tag lives only in the clone.
git clone -q --no-hardlinks "$REPO_ROOT" "$TMP/clone"
git -C "$TMP/clone" tag "$TAG"
export SDK_REPO="$TMP/clone"

# 2. Scratch copy of the fixture service without generated stubs: the script must
# produce them itself (buf generate), as a fresh checkout has none.
SVC="$TMP/sync-fixture"
cp -R "$SCRIPT_DIR/testdata/sync-fixture" "$SVC"
rm -rf "$SVC/generated"
SVC_MOD="$(awk '$1=="module"{print $2; exit}' "$SVC/go.mod")"

# 3. Sync vendors the tagged tree.
bash "$SYNC" "$TAG" "$SVC" >"$TMP/out.log" 2>&1 || { cat "$TMP/out.log"; fail "sync with an existing tag failed"; }
[ -f "$SVC/third_party/go-sdk/VERSION" ] || fail "VERSION missing"
grep -q "$TAG" "$SVC/third_party/go-sdk/VERSION" || fail "VERSION does not name the tag"
COMMIT="$(git -C "$TMP/clone" rev-parse "$TAG^{commit}")"
grep -q "$COMMIT" "$SVC/third_party/go-sdk/VERSION" || fail "VERSION does not name the commit"
grep -Eq "replace.*$SDK_MOD => \./third_party/go-sdk|^[[:space:]]*$SDK_MOD => \./third_party/go-sdk" "$SVC/go.mod" \
  || fail "go.mod has no replace for the SDK"
[ -f "$SVC/third_party/go-sdk/go.mod" ] || fail "SDK module not vendored"
[ -f "$SVC/make/go-service.mk" ] || fail "go-service.mk not copied"
if git -C "$TMP/clone" cat-file -e "$TAG:platform-core/packages/go-sdk/.golangci.yml" 2>/dev/null; then
  [ -f "$SVC/.golangci.yml" ] || fail ".golangci.yml present at tag but not copied"
else
  grep -qi "warn.*golangci" "$TMP/out.log" || fail "absent .golangci.yml should produce a warning"
  [ ! -f "$SVC/.golangci.yml" ] || fail ".golangci.yml appeared although absent at tag"
fi
[ -d "$SVC/generated" ] || fail "sync did not generate the proto stubs"
(cd "$SVC" && go build ./...) || fail "go build ./... fails after sync"
pass "sync vendors the tagged tree and generates stubs"

# 3a. local-prefixes in the synced lint config is the service module.
if [ -f "$SVC/.golangci.yml" ]; then
  grep -A1 'local-prefixes:' "$SVC/.golangci.yml" | grep -q -- "- $SVC_MOD\$" \
    || fail "local-prefixes is not the service module $SVC_MOD"
  grep -q 'platform-core$' "$SVC/.golangci.yml" && fail "platform-core prefix left in .golangci.yml"
  pass "local-prefixes set to $SVC_MOD"
fi

# 3b. The lint tool is available in the service at the SDK's pinned version.
LINT_PKG="github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
if grep -q "^tool $LINT_PKG" "$SVC/third_party/go-sdk/go.mod"; then
  grep -q "^tool $LINT_PKG" "$SVC/go.mod" || fail "service go.mod has no tool directive for golangci-lint"
  want="$(awk '$1=="github.com/golangci/golangci-lint/v2"{print $2}' "$SVC/third_party/go-sdk/go.mod")"
  got="$(awk '$1=="github.com/golangci/golangci-lint/v2"{print $2}' "$SVC/go.mod")"
  [ -n "$want" ] && [ "$want" = "$got" ] || fail "golangci-lint version: SDK pins '$want', service has '$got'"
  (cd "$SVC" && go tool golangci-lint version >/dev/null) || fail "go tool golangci-lint version fails in the service"
  pass "golangci-lint tool pinned at $want"
else
  echo "skip: tag has no golangci-lint tool directive"
fi

# 4. A second sync of the same tag replaces the copy cleanly.
bash "$SYNC" "$TAG" "$SVC" >"$TMP/out2.log" 2>&1 || { cat "$TMP/out2.log"; fail "second sync failed"; }
[ "$(grep -c "=> ./third_party/go-sdk" "$SVC/go.mod")" = 1 ] || fail "replace line duplicated after second sync"
[ "$(grep -c "^tool $LINT_PKG" "$SVC/go.mod")" -le 1 ] || fail "tool directive duplicated after second sync"
ls "$SVC/third_party" | grep -qv '^go-sdk$' && fail "stray files left in third_party"
pass "second sync is idempotent"

# 5. Unknown tag: non-zero exit, tree unchanged.
before="$(tree_sum "$SVC")"
if bash "$SYNC" "$MISSING_TAG" "$SVC" >"$TMP/out3.log" 2>&1; then
  fail "sync with a missing tag exited zero"
fi
[ "$(tree_sum "$SVC")" = "$before" ] || fail "service tree changed after a failed sync"
pass "unknown tag changes nothing"

# 6. Fresh service without a prior copy: unknown tag leaves it untouched too.
FRESH="$TMP/fresh"
cp -R "$SCRIPT_DIR/testdata/sync-fixture" "$FRESH"
rm -rf "$FRESH/third_party"
before="$(tree_sum "$FRESH")"
if bash "$SYNC" "$MISSING_TAG" "$FRESH" >/dev/null 2>&1; then fail "missing tag exited zero on fresh service"; fi
[ "$(tree_sum "$FRESH")" = "$before" ] || fail "fresh service tree changed after a failed sync"
[ ! -e "$FRESH/third_party" ] || fail "third_party created by a failed sync"
pass "unknown tag leaves a fresh service untouched"

# 7. Service-local lint block survives a re-sync (byte for byte), the result
# still verifies, and an absent block or file is handled.
if [ -f "$SVC/.golangci.yml" ]; then
  grep -q '# BEGIN service-local' "$SVC/.golangci.yml" || fail "synced .golangci.yml has no service-local block"
  python3 - "$SVC/.golangci.yml" <<'PY' || fail "could not edit the service-local block"
import sys
p = sys.argv[1]
s = open(p).read()
add = "      - path: internal/legacy/\n        linters:\n          - revive\n      # a comment  with  odd   spacing\t\n"
s = s.replace("# BEGIN service-local\n", "# BEGIN service-local\n" + add, 1)
open(p, "w").write(s)
PY
  block_of() { awk '/# BEGIN service-local/{f=1;next} /# END service-local/{f=0} f' "$1"; }
  block_of "$SVC/.golangci.yml" > "$TMP/block.before"
  [ -s "$TMP/block.before" ] || fail "test block is empty"
  bash "$SYNC" "$TAG" "$SVC" >"$TMP/out4.log" 2>&1 || { cat "$TMP/out4.log"; fail "re-sync with a service-local block failed"; }
  block_of "$SVC/.golangci.yml" > "$TMP/block.after"
  cmp -s "$TMP/block.before" "$TMP/block.after" || fail "service-local block changed after re-sync"
  [ "$(grep -c '# BEGIN service-local' "$SVC/.golangci.yml")" = 1 ] || fail "service-local markers duplicated"
  (cd "$SVC" && go tool golangci-lint config verify) || fail "golangci-lint config verify fails on the re-synced config"
  grep -A1 'local-prefixes:' "$SVC/.golangci.yml" | grep -q -- "- $SVC_MOD\$" || fail "local-prefixes lost after re-sync"
  # Absent file: a first sync still produces the empty block.
  rm "$SVC/.golangci.yml"
  bash "$SYNC" "$TAG" "$SVC" >"$TMP/out5.log" 2>&1 || { cat "$TMP/out5.log"; fail "sync without a .golangci.yml failed"; }
  [ -z "$(block_of "$SVC/.golangci.yml")" ] || fail "fresh sync block is not empty"
  (cd "$SVC" && go tool golangci-lint config verify) || fail "config verify fails on the first-sync config"
  pass "service-local lint block is preserved"
fi

# 8. A fresh service that does not import the SDK keeps the go-sdk require
# through go mod tidy, so the first SDK import builds without editing go.mod.
# A second sync leaves exactly one require.
NEW="$TMP/fresh-require"
mkdir -p "$NEW"
printf 'module example.com/fresh\n\ngo 1.25\n' > "$NEW/go.mod"
printf 'package main\n\nfunc main() {}\n' > "$NEW/main.go"
bash "$SYNC" "$TAG" "$NEW" >"$TMP/out6.log" 2>&1 || { cat "$TMP/out6.log"; fail "sync of a service without SDK imports failed"; }
grep -Eq "^[[:space:]]*(require[[:space:]]+)?$SDK_MOD[[:space:]]+v0\.0\.0-00010101000000-000000000000" "$NEW/go.mod" \
  || fail "go.mod lost the go-sdk require after sync"
grep -q "$SDK_MOD => ./third_party/go-sdk" "$NEW/go.mod" || fail "go.mod has no replace for the SDK"
mkdir -p "$NEW/internal/uses"
printf 'package uses\n\nimport _ "%s/pkg/reqctx"\n' "$SDK_MOD" > "$NEW/internal/uses/uses.go"
cp "$NEW/go.mod" "$TMP/fresh.go.mod.before"
(cd "$NEW" && go build ./...) || fail "go build fails after adding an SDK import"
cmp -s "$NEW/go.mod" "$TMP/fresh.go.mod.before" || fail "building with an SDK import edited go.mod"
bash "$SYNC" "$TAG" "$NEW" >"$TMP/out7.log" 2>&1 || { cat "$TMP/out7.log"; fail "second sync of the fresh service failed"; }
[ "$(grep -c "$SDK_MOD v0.0.0-00010101000000-000000000000" "$NEW/go.mod")" = 1 ] || fail "require duplicated after second sync"
pass "sync keeps the go-sdk require for a service without SDK imports"

# 9. A service that already imports the SDK gets no internal/sdkimport; the
# fresh service of test 8 keeps its own.
[ -f "$NEW/internal/sdkimport/sdkimport.go" ] || fail "fresh service has no internal/sdkimport"
USES="$TMP/uses-sdk"
mkdir -p "$USES/internal/app"
printf 'module example.com/uses\n\ngo 1.25\n' > "$USES/go.mod"
printf 'package app\n\nimport _ "%s/pkg/reqctx"\n' "$SDK_MOD" > "$USES/internal/app/app.go"
bash "$SYNC" "$TAG" "$USES" >"$TMP/out8.log" 2>&1 || { cat "$TMP/out8.log"; fail "sync of a service that imports the SDK failed"; }
[ ! -e "$USES/internal/sdkimport" ] || fail "internal/sdkimport created for a service that imports the SDK"
grep -q "$SDK_MOD v0.0.0-00010101000000-000000000000" "$USES/go.mod" || fail "go.mod lost the go-sdk require"
(cd "$USES" && go build ./...) || fail "go build fails for the service that imports the SDK"
pass "no sdkimport package for a service that imports the SDK"

echo "all sync-go-sdk tests passed"
