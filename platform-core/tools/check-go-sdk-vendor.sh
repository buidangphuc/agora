#!/usr/bin/env bash
# ==============================================================================
# CHECK THE VENDORED GO SDK AGAINST ITS TAG (ADR-0016)
# ==============================================================================
# Usage: check-go-sdk-vendor.sh [root]
#
# For every <root>/team-*/third_party/go-sdk/VERSION, exports packages/go-sdk at
# the tag named in VERSION (tag=<tag>) and diffs it against the vendored copy.
# Names each service and file that differs, is missing or is extra, and exits 1
# if any do. No such service is fine (exit 0). VERSION itself is not compared.
#
# root defaults to the repo top level. SDK_REPO overrides the repo that holds
# the tags (default: this repo). CI must have fetched tags (git fetch --tags).
# ==============================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO="${SDK_REPO:-$(git -C "$CORE_DIR" rev-parse --show-toplevel)}"
ROOT="${1:-$(git -C "$CORE_DIR" rev-parse --show-toplevel)}"
SDK_PATH="platform-core/packages/go-sdk"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/check-go-sdk-vendor.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

status=0
checked=0
for version in "$ROOT"/team-*/third_party/go-sdk/VERSION; do
  [ -f "$version" ] || continue
  vendored="$(dirname "$version")"
  svc="$(basename "$(dirname "$(dirname "$vendored")")")"
  checked=$((checked + 1))

  tag="$(sed -n 's/^tag=//p' "$version" | head -n 1)"
  if [ -z "$tag" ]; then
    echo "$svc: VERSION has no tag= line"; status=1; continue
  fi

  want="$WORK/$svc"
  mkdir -p "$want"
  if ! git -C "$REPO" archive --format=tar "$tag:$SDK_PATH" 2>/dev/null | tar -x -C "$want" 2>/dev/null \
      || [ -z "$(ls -A "$want")" ]; then
    echo "$svc: tag '$tag' not found in $REPO (or has no $SDK_PATH); fetch tags?"
    status=1; continue
  fi

  # diff -rq prints "Files A/x and B/x differ" and "Only in <dir>: <name>".
  out="$(diff -rq -x VERSION "$want" "$vendored" 2>&1)"
  if [ $? -ne 0 ]; then
    status=1
    echo "$svc: vendored go-sdk differs from $tag"
    while IFS= read -r line; do
      case "$line" in
        "Files $want/"*" and "*" differ")
          rel="${line#Files "$want"/}"; echo "  differs: ${rel%% and *}" ;;
        "Only in $want"*)
          dir="${line#Only in }"; dir="${dir%%: *}"; name="${line##*: }"
          echo "  missing: ${dir#"$want"}/$name" | sed 's#: //#: #' ;;
        "Only in $vendored"*)
          dir="${line#Only in }"; dir="${dir%%: *}"; name="${line##*: }"
          echo "  extra: ${dir#"$vendored"}/$name" | sed 's#: //#: #' ;;
        *) echo "  $line" ;;
      esac
    done <<< "$out"
  else
    echo "$svc: ok ($tag)"
  fi
done

[ "$checked" -gt 0 ] || echo "no vendored go-sdk found under $ROOT/team-*; nothing to check"
exit "$status"
