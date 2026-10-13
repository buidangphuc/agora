#!/usr/bin/env bash
# Test for tools/check-go-sdk-vendor.sh. Runs against a throwaway clone of this
# repo (tags are created there, never in the real repo) and a scratch root with
# fake team-* services. Run: bash platform-core/tools/check-go-sdk-vendor_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
CHECK="$SCRIPT_DIR/check-go-sdk-vendor.sh"
TAG="go-sdk/v0.0.0-test"
SDK_PATH="platform-core/packages/go-sdk"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/check-go-sdk-vendor-test.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok: $*"; }

[ -f "$CHECK" ] || fail "script missing: $CHECK"

git clone -q --no-hardlinks "$REPO_ROOT" "$TMP/clone"
git -C "$TMP/clone" tag "$TAG"
COMMIT="$(git -C "$TMP/clone" rev-parse "$TAG^{commit}")"
export SDK_REPO="$TMP/clone"

# vendor <root> <service>: a service with a pristine copy of the tagged SDK.
vendor() {
  local dir="$1/$2/third_party/go-sdk"
  mkdir -p "$dir"
  git -C "$TMP/clone" archive --format=tar "$TAG:$SDK_PATH" | tar -x -C "$dir"
  printf 'tag=%s\ncommit=%s\n' "$TAG" "$COMMIT" > "$dir/VERSION"
}

run() { bash "$CHECK" "$1" >"$TMP/out.log" 2>&1; }

# 1. No team-*/third_party/go-sdk/VERSION at all: nothing to check, exit 0.
mkdir -p "$TMP/empty/team-none"
run "$TMP/empty" || { cat "$TMP/out.log"; fail "a root with no vendored SDK must pass"; }
pass "zero vendored services exits 0"

# 2. Two pristine services pass.
R="$TMP/root"
vendor "$R" team-a
vendor "$R" team-b
run "$R" || { cat "$TMP/out.log"; fail "pristine vendored copies must pass"; }
pass "pristine copies exit 0"

# 3. An edited file is named with its service; the clean one is not.
echo "// tampered" >> "$R/team-a/third_party/go-sdk/pkg/auth/doc.go"
if run "$R"; then fail "an edited vendored file must fail"; fi
grep -q "team-a" "$TMP/out.log" || fail "output does not name the service team-a"
grep -q "pkg/auth/doc.go" "$TMP/out.log" || fail "output does not name the edited file"
grep -q "^team-b: vendored" "$TMP/out.log" && fail "clean service team-b must not be reported"
pass "edited file is reported with its service"

# 4. An extra and a missing file are both reported.
rm -rf "$R/team-a/third_party/go-sdk" && vendor "$R" team-a
echo x > "$R/team-b/third_party/go-sdk/stray.txt"
rm "$R/team-b/third_party/go-sdk/go.mod"
if run "$R"; then fail "extra or missing files must fail"; fi
grep -q "team-b" "$TMP/out.log" || fail "output does not name team-b"
grep -q "stray.txt" "$TMP/out.log" || fail "extra file not named"
grep -q "go.mod" "$TMP/out.log" || fail "missing file not named"
grep -q "^team-a: vendored" "$TMP/out.log" && fail "restored service team-a must not be reported"
pass "extra and missing files are reported"

# 5. A tag that does not exist fails and names the service.
rm -rf "$R/team-b/third_party/go-sdk" && vendor "$R" team-b
printf 'tag=go-sdk/v9.9.9-missing\ncommit=0\n' > "$R/team-b/third_party/go-sdk/VERSION"
if run "$R"; then fail "an unknown tag must fail"; fi
grep -q "team-b" "$TMP/out.log" || fail "unknown tag: output does not name team-b"
pass "unknown tag fails"

echo "all checks passed"
