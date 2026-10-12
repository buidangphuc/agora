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
#   4. ensures go.mod has the replace to ./third_party/go-sdk
#   5. runs go mod tidy
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
MK_SRC="$CORE_DIR/make/go-service.mk"

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
      && (cd "$svc" && go mod edit -replace="$SDK_MOD=./third_party/go-sdk" && go mod tidy); }; then
    echo "error: sync failed for $svc; restoring previous state" >&2
    rm -rf "$stage"
    restore
    return 1
  fi
}

sync_lint_config() {
  if [ -f "$1/third_party/go-sdk/.golangci.yml" ]; then
    cp "$1/third_party/go-sdk/.golangci.yml" "$1/.golangci.yml"
  else
    echo "warning: $TAG has no .golangci.yml in the SDK; skipping it" >&2
  fi
}

for svc in "${SERVICES[@]}"; do
  sync_one "$svc"
  echo "  ✓ $(basename "$svc") now at $TAG"
done
