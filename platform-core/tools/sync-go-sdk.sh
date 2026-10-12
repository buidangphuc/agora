#!/usr/bin/env bash
# ==============================================================================
# SYNC THE GO PLATFORM SDK INTO SERVICES (ADR-0016)
# ==============================================================================
# Usage: sync-go-sdk.sh <tag> <service-dir>...
#
# For each service directory:
#   1. exports packages/go-sdk at git tag <tag> into <service>/third_party/go-sdk
#      (replacing any previous copy) and writes third_party/go-sdk/VERSION
#   2. copies make/go-service.mk to <service>/make/go-service.mk
#   3. copies .golangci.yml from the SDK at the tag to <service>/.golangci.yml
#      (skipped with a warning when the tag has no such file)
#      and points its goimports local-prefixes at the service's module path;
#      the lines between `# BEGIN service-local` and `# END service-local` in
#      the service's existing file are kept byte for byte
#   4. ensures go.mod has the replace to ./third_party/go-sdk
#   5. adds a `tool` directive for golangci-lint at the version the SDK pins
#      (go get -tool), so `go tool golangci-lint` works; skipped with a
#      warning when the SDK at the tag has no such directive
#   6. runs `buf generate` when the service has a buf.gen.yaml, because the
#      generated stubs are not committed and go mod tidy needs them
#   7. runs go mod tidy
#
# The tag and every service directory are checked before anything changes. If a
# step fails after that, the service is restored to its previous state.
# SDK_REPO overrides the repo that holds the tags (default: this repo).
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO="${SDK_REPO:-$(git -C "$CORE_DIR" rev-parse --show-toplevel)}"
SDK_PATH="platform-core/packages/go-sdk"
SDK_MOD="github.com/buidangphuc/platform-core/packages/go-sdk"
SDK_PREFIX="github.com/buidangphuc/platform-core"
LINT_MOD="github.com/golangci/golangci-lint/v2"
LINT_PKG="$LINT_MOD/cmd/golangci-lint"
MK_SRC="$CORE_DIR/make/go-service.mk"
LOCAL_BEGIN="# BEGIN service-local"
LOCAL_END="# END service-local"

usage() { echo "usage: $(basename "$0") <tag> <service-dir>..." >&2; exit 2; }
die() { echo "error: $*" >&2; exit 1; }

[ $# -ge 2 ] || usage
TAG="$1"; shift

# --- validate everything before touching anything -----------------------------
COMMIT="$(git -C "$REPO" rev-parse --verify "$TAG^{commit}" 2>/dev/null)" \
  || die "tag '$TAG' not found in $REPO"
git -C "$REPO" cat-file -e "$TAG:$SDK_PATH/go.mod" 2>/dev/null \
  || die "tag '$TAG' has no $SDK_PATH"
[ -f "$MK_SRC" ] || die "missing $MK_SRC"

SERVICES=()
for dir in "$@"; do
  [ -f "$dir/go.mod" ] || die "'$dir' is not a Go service directory (no go.mod)"
  SERVICES+=("$(cd "$dir" && pwd)")
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/sync-go-sdk.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

sync_one() {
  local svc="$1" tp="$1/third_party" stage backup="$WORK/backup.$RANDOM"
  echo "· $(basename "$svc"): vendoring $TAG ($COMMIT)"

  # Export the tag into a staging dir on the same filesystem as the target.
  mkdir -p "$tp"
  stage="$(mktemp -d "$tp/.go-sdk.new.XXXXXX")"
  git -C "$REPO" archive --format=tar "$TAG:$SDK_PATH" | tar -x -C "$stage"
  printf 'tag=%s\ncommit=%s\n' "$TAG" "$COMMIT" > "$stage/VERSION"

  # Save what we are about to change so a failure can undo it.
  mkdir -p "$backup"
  cp "$svc/go.mod" "$backup/go.mod"
  [ ! -f "$svc/go.sum" ] || cp "$svc/go.sum" "$backup/go.sum"
  [ ! -f "$svc/.golangci.yml" ] || cp "$svc/.golangci.yml" "$backup/golangci.yml"
  [ ! -f "$svc/make/go-service.mk" ] || cp "$svc/make/go-service.mk" "$backup/go-service.mk"
  local had_old=0
  if [ -d "$tp/go-sdk" ]; then had_old=1; mv "$tp/go-sdk" "$backup/go-sdk"; fi

  restore() {
    rm -rf "$tp/go-sdk"
    [ "$had_old" = 0 ] || mv "$backup/go-sdk" "$tp/go-sdk"
    cp "$backup/go.mod" "$svc/go.mod"
    [ ! -f "$backup/go.sum" ] || cp "$backup/go.sum" "$svc/go.sum"
    for pair in "golangci.yml:.golangci.yml" "go-service.mk:make/go-service.mk"; do
      if [ -f "$backup/${pair%%:*}" ]; then cp "$backup/${pair%%:*}" "$svc/${pair##*:}"
      else rm -f "$svc/${pair##*:}"; fi
    done
    [ -n "$(ls -A "$tp" 2>/dev/null)" ] || rmdir "$tp" 2>/dev/null || true
  }

  # Swap in the new copy, then update the rest. Any failure restores the old state.
  if ! { mv "$stage" "$tp/go-sdk" \
      && mkdir -p "$svc/make" && cp "$MK_SRC" "$svc/make/go-service.mk" \
      && sync_lint_config "$svc" \
      && (cd "$svc" && go mod edit -replace="$SDK_MOD=./third_party/go-sdk") \
      && sync_lint_tool "$svc" \
      && sync_generate "$svc" \
      && (cd "$svc" && go mod tidy); }; then
    echo "error: sync failed for $svc; restoring previous state" >&2
    rm -rf "$stage"
    restore
    return 1
  fi
}

sync_lint_config() {
  if [ -f "$1/third_party/go-sdk/.golangci.yml" ]; then
    local mod
    mod="$(awk '$1=="module"{print $2; exit}' "$1/go.mod")"
    [ -n "$mod" ] || { echo "error: no module line in $1/go.mod" >&2; return 1; }
    # The SDK's own prefix is the module that import groups should separate out.
    local out="$WORK/golangci.$RANDOM"
    sed "s#^\([[:space:]]*- \)$SDK_PREFIX\$#\1$mod#" \
      "$1/third_party/go-sdk/.golangci.yml" > "$out"
    # Keep the service-local block of the previous file (none on a first sync).
    if [ -f "$1/.golangci.yml" ] && grep -q "$LOCAL_BEGIN" "$1/.golangci.yml"; then
      if grep -q "$LOCAL_BEGIN" "$out"; then
        awk -v old="$1/.golangci.yml" -v b="$LOCAL_BEGIN" -v e="$LOCAL_END" '
          BEGIN { f = 0; while ((getline l < old) > 0) {
                    if (index(l, e)) f = 0
                    if (f) keep[n++] = l
                    if (index(l, b)) f = 1 }
                  f = 0 }
          index($0, b) { print; for (i = 0; i < n; i++) print keep[i]; f = 1; next }
          index($0, e) { f = 0 }
          !f { print }
        ' "$out" > "$out.merged" && mv "$out.merged" "$out"
      else
        echo "warning: $TAG has no service-local block; the old one is dropped" >&2
      fi
    fi
    mv "$out" "$1/.golangci.yml"
  else
    echo "warning: $TAG has no .golangci.yml in the SDK; skipping it" >&2
  fi
}

# Pin golangci-lint in the service at the version the vendored SDK uses.
sync_lint_tool() {
  local gomod="$1/third_party/go-sdk/go.mod" ver
  if ! grep -q "^tool $LINT_PKG\$" "$gomod"; then
    echo "warning: $TAG has no golangci-lint tool directive; skipping it" >&2
    return 0
  fi
  ver="$(awk -v m="$LINT_MOD" '$1==m {print $2; exit}' "$gomod")"
  [ -n "$ver" ] || { echo "error: $LINT_MOD not required in SDK go.mod" >&2; return 1; }
  (cd "$1" && go get -tool "$LINT_PKG@$ver")
}

# Generate the proto stubs (gitignored, absent in a fresh checkout).
sync_generate() {
  [ -f "$1/buf.gen.yaml" ] || return 0
  command -v buf >/dev/null || { echo "error: buf not found; install it to generate stubs" >&2; return 1; }
  (cd "$1" && buf generate)
}

for svc in "${SERVICES[@]}"; do
  sync_one "$svc"
  echo "  ✓ $(basename "$svc") now at $TAG"
done
