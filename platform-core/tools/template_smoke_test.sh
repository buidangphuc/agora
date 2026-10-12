#!/usr/bin/env bash
# Smoke test for templates/_service-go: copy it, fill the placeholders, vendor
# the SDK from a throwaway clone's tag, and make sure it builds and vets. Needs
# no buf: the template registers a hand-built ServiceDesc.
# Run: bash platform-core/tools/template_smoke_test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
TEMPLATE="$SCRIPT_DIR/../templates/_service-go"
SYNC="$SCRIPT_DIR/sync-go-sdk.sh"
TAG="go-sdk/v0.0.0-test"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/template-smoke-test.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok: $*"; }

[ -d "$TEMPLATE" ] || fail "template missing: $TEMPLATE"

git clone -q --no-hardlinks "$REPO_ROOT" "$TMP/clone"
git -C "$TMP/clone" tag "$TAG"
export SDK_REPO="$TMP/clone"

# 1. Instantiate: copy, then replace the two placeholders everywhere.
SVC="$TMP/team-demo"
cp -R "$TEMPLATE" "$SVC"
find "$SVC" -name '*.tmpl' | while IFS= read -r f; do mv "$f" "${f%.tmpl}"; done
[ -z "$(find "$SVC" -name '*.tmpl')" ] || fail ".tmpl files left after rename"
{ grep -rIl -e "__SERVICE__" -e "__PORT__" "$SVC" || true; } | while IFS= read -r f; do
  sed -e 's/__SERVICE__/team-demo/g' -e 's/__PORT__/50099/g' "$f" > "$f.new" && mv "$f.new" "$f"
done
grep -rI -e '__SERVICE__' -e '__PORT__' "$SVC" >/dev/null && fail "placeholders left after substitution"
grep -rIq 'team-demo' "$SVC/go.mod" || fail "go.mod does not carry the service name"
grep -q '50099' "$SVC/Makefile" || fail "Makefile does not carry the port"
[ ! -e "$SVC/third_party" ] || fail "template must not ship third_party/"
[ ! -e "$SVC/generated" ] || fail "template must not ship generated/"
[ -f "$SVC/README.md" ] || fail "template has no README"
pass "template instantiates"

# 2. Sync the SDK from the tag, as the README tells the reader to.
bash "$SYNC" "$TAG" "$SVC" >"$TMP/sync.log" 2>&1 || { cat "$TMP/sync.log"; fail "sync-go-sdk.sh failed on the template"; }
[ -f "$SVC/third_party/go-sdk/VERSION" ] || fail "VERSION missing after sync"
pass "sync vendors the SDK"

# 3. It compiles and vets, and carries no hand-rolled interceptor or server.
(cd "$SVC" && go build ./... && go vet ./...) || fail "go build/vet fails on the instantiated template"
[ ! -e "$SVC/internal/interceptor" ] || fail "internal/interceptor must not exist (the SDK owns it)"
[ ! -e "$SVC/internal/grpcserver" ] || fail "internal/grpcserver must not exist (the SDK owns it)"
pass "go build and go vet pass; no local interceptor or grpcserver"

echo "all checks passed"
