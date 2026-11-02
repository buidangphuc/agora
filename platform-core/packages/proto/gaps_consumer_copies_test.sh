#!/usr/bin/env bash
# Task 1.4.1 - capability "Gaps contract". Scenarios:
#   "Promotion copies match the source" / "A drifted or missing promotion copy is rejected"
#   "Identity copies match the source" / "A drifted or missing identity copy is rejected"
#   "Audit copies match the source" / "A drifted or missing audit copy is rejected"
# Usage: [REPO_ROOT=...] bash platform-core/packages/proto/gaps_consumer_copies_test.sh
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"
MIN_promotion=7
MIN_identity=13
MIN_audit=13

consumers() {
  local root="$1" dom="$2" f
  for f in "$root"/team-*/proto/platform/"$dom"/v1/"$dom".proto; do
    [ -e "$f" ] && echo "$f"
  done
}

check_copies() {
  local root="$1" dom="$2" rc=0 f
  [ -f "$root/$SRC_REL/$dom/v1/$dom.proto" ] || { echo "FAIL source missing: $dom"; return 1; }
  for f in $(consumers "$root" "$dom"); do
    cmp -s "$root/$SRC_REL/$dom/v1/$dom.proto" "$f" || { echo "FAIL copies-match: ${f#$root/}"; rc=1; }
  done
  return $rc
}

check_missing() {
  local root="$1" dom="$2" rc=0 d
  for d in "$root"/team-*/proto/platform/"$dom"/v1; do
    [ -d "$d" ] || continue
    [ -f "$d/$dom.proto" ] || { echo "FAIL missing copy: ${d#$root/}/$dom.proto"; rc=1; }
  done
  return $rc
}

check_all() { check_copies "$1" "$2" && check_missing "$1" "$2"; }

fail=0
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
SYN_REPOS="team-a team-b team-c"

mk() {
  local dom="$1" r
  rm -rf "$tmp/t"; mkdir -p "$tmp/t/$SRC_REL/$dom/v1"
  printf '%s source\n' "$dom" > "$tmp/t/$SRC_REL/$dom/v1/$dom.proto"
  for r in $SYN_REPOS; do
    mkdir -p "$tmp/t/$r/proto/platform/$dom/v1"
    cp "$tmp/t/$SRC_REL/$dom/v1/$dom.proto" "$tmp/t/$r/proto/platform/$dom/v1/$dom.proto"
  done
}

for dom in promotion identity audit; do
  min="MIN_$dom"; min="${!min}"
  n="$(consumers "$ROOT" "$dom" | wc -l | tr -d ' ')"
  if [ "$n" -ge "$min" ]; then echo "ok   $dom-copies-match: discovered $n consumers"; else echo "FAIL $dom-copies-match: discovered $n consumers, want >= $min"; fail=1; fi
  if out="$(check_all "$ROOT" "$dom")"; then echo "ok   $dom-copies-match: all copies equal source"; else echo "$out"; fail=1; fi

  mk "$dom"
  check_all "$tmp/t" "$dom" >/dev/null && echo "ok   $dom faithful tree accepted" || { echo "FAIL $dom-drift-rejected: faithful synthetic tree rejected"; fail=1; }
  mk "$dom"; printf 'x' >> "$tmp/t/team-b/proto/platform/$dom/v1/$dom.proto"
  check_all "$tmp/t" "$dom" >/dev/null && { echo "FAIL $dom-drift-rejected: one-byte drift accepted"; fail=1; } || echo "ok   $dom one-byte drift rejected"
  mk "$dom"; rm "$tmp/t/team-c/proto/platform/$dom/v1/$dom.proto"
  check_all "$tmp/t" "$dom" >/dev/null && { echo "FAIL $dom-drift-rejected: missing copy accepted"; fail=1; } || echo "ok   $dom missing copy rejected"
  mk "$dom"; mkdir -p "$tmp/t/team-d/proto/platform/$dom/v1"
  check_all "$tmp/t" "$dom" >/dev/null && { echo "FAIL $dom-drift-rejected: newly globbed empty consumer dir accepted"; fail=1; } || echo "ok   $dom glob-discovered missing copy rejected"
done
exit $fail
