#!/usr/bin/env bash
# Test for tools/sync-go-sdk.sh. Runs against a throwaway clone of this repo
# (so tags are created there, never in the real repo) and a scratch copy of
# team-sharing. Run: bash platform-core/tools/sync-go-sdk_test.sh
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

# 2. Scratch copy of team-sharing (+ gitignored generated stubs when present).
SVC="$TMP/team-sharing"
cp -R "$REPO_ROOT/team-sharing" "$SVC"
if [ ! -d "$SVC/generated" ]; then
  COMMON="$(cd "$REPO_ROOT" && cd "$(git rev-parse --git-common-dir)/.." && pwd)"
  if [ -d "$COMMON/team-sharing/generated" ]; then
    cp -R "$COMMON/team-sharing/generated" "$SVC/generated"
  else
    (cd "$SVC" && buf generate) || fail "cannot obtain generated stubs for team-sharing"
  fi
fi
(cd "$SVC" && go build ./...) || fail "scratch team-sharing does not build before sync"

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
(cd "$SVC" && go build ./...) || fail "go build ./... fails after sync"
pass "sync vendors the tagged tree"

# 4. A second sync of the same tag replaces the copy cleanly.
bash "$SYNC" "$TAG" "$SVC" >"$TMP/out2.log" 2>&1 || { cat "$TMP/out2.log"; fail "second sync failed"; }
[ "$(grep -c "=> ./third_party/go-sdk" "$SVC/go.mod")" = 1 ] || fail "replace line duplicated after second sync"
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
cp -R "$REPO_ROOT/team-sharing" "$FRESH"
before="$(tree_sum "$FRESH")"
if bash "$SYNC" "$MISSING_TAG" "$FRESH" >/dev/null 2>&1; then fail "missing tag exited zero on fresh service"; fi
[ "$(tree_sum "$FRESH")" = "$before" ] || fail "fresh service tree changed after a failed sync"
[ ! -e "$FRESH/third_party" ] || fail "third_party created by a failed sync"
pass "unknown tag leaves a fresh service untouched"

echo "all sync-go-sdk tests passed"
