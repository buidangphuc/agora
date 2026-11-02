#!/usr/bin/env bash
# Tasks 1.4.1 and 1.4.2 - capability "Gaps contract". Scenarios:
#   "Promotion copies match the source" / "A drifted or missing promotion copy is rejected"
#   "Identity copies match the source" / "A drifted or missing identity copy is rejected"
#   "Audit copies match the source" / "A drifted or missing audit copy is rejected"
#   "Generated code compiles in Go consumers" (promotion-go-builds)
#   "Generated TypeScript compiles in frontend consumers" (promotion-ts-builds)
#   "Generated code compiles in Go and TypeScript consumers" (identity-builds)
#   "Lint and breaking pass" (buf-lint-breaking) / "Regeneration is reproducible" (regen-reproducible)
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
BUF="${BUF:-buf}"
export PATH="$HOME/go/bin:$PATH"

gen_go() {
  local d="$1" t
  t="$tmp/gen-$(basename "$d").yaml"
  sed -E -e 's#remote: buf.build/protocolbuffers/go#local: protoc-gen-go#' \
      -e 's#remote: buf.build/grpc/go#local: protoc-gen-go-grpc#' \
      -e 's#remote: buf.build/connectrpc/go.*#local: protoc-gen-connect-go#' \
      "$d/buf.gen.yaml" > "$t" || return 1
  (cd "$d" && $BUF generate --template "$t") >"$tmp/gen.log" 2>&1
}

gen_ts() {
  local d="$1"
  (cd "$d" && PATH="$d/node_modules/.bin:$PATH" $BUF generate) >"$tmp/gen.log" 2>&1
}

dom_repos() {
  local dom="$1" kind="$2" f r
  for f in $(consumers "$ROOT" "$dom"); do
    r="${f%/proto/platform/*}"
    case "$kind" in
      go) [ -f "$r/go.mod" ] && echo "$r" ;;
      ts) [ -f "$r/package.json" ] && echo "$r" ;;
    esac
  done
}

has_all() {
  local dir="$1"; shift
  local sym
  for sym in "$@"; do
    grep -rqw "$sym" "$dir" 2>/dev/null || { echo "missing symbol $sym under ${dir#$ROOT/}"; return 1; }
  done
}

go_build_check() {
  local check="$1" dom="$2"; shift 2
  local r n=0 bad=0 out
  for r in $(dom_repos "$dom" go); do
    n=$((n+1))
    if ! gen_go "$r"; then echo "FAIL $check: regenerate ${r#$ROOT/}: $(head -3 "$tmp/gen.log" | tr '\n' ' ')"; bad=1; continue; fi
    if ! (cd "$r" && go build ./... >"$tmp/b.log" 2>&1); then echo "FAIL $check: go build ${r#$ROOT/}: $(head -3 "$tmp/b.log" | tr '\n' ' ')"; bad=1; continue; fi
    out="$(has_all "$r/generated/platform/$dom/v1" "$@")" || { echo "FAIL $check: ${r#$ROOT/}: $out"; bad=1; }
  done
  [ "$n" -gt 0 ] || { echo "FAIL $check: no Go consumers discovered"; return 1; }
  [ "$bad" -eq 0 ] && echo "ok   $check: $n Go consumers build"
  return $bad
}

ts_build_check() {
  local check="$1" dom="$2"; shift 2
  local r n=0 bad=0 out
  for r in $(dom_repos "$dom" ts); do
    n=$((n+1))
    if ! gen_ts "$r"; then echo "FAIL $check: regenerate ${r#$ROOT/}: $(head -3 "$tmp/gen.log" | tr '\n' ' ')"; bad=1; continue; fi
    if ! (cd "$r" && PATH="$r/node_modules/.bin:$PATH" npm run typecheck >"$tmp/b.log" 2>&1); then echo "FAIL $check: typecheck ${r#$ROOT/}: $(tail -3 "$tmp/b.log" | tr '\n' ' ')"; bad=1; continue; fi
    out="$(has_all "$r/src/generated/platform/$dom/v1" "$@")" || { echo "FAIL $check: ${r#$ROOT/}: $out"; bad=1; }
  done
  [ "$n" -gt 0 ] || { echo "FAIL $check: no frontend consumers discovered"; return 1; }
  [ "$bad" -eq 0 ] && echo "ok   $check: $n frontend consumers typecheck"
  return $bad
}

out="$(go_build_check promotion-go-builds promotion DeactivateVoucher UpdateVoucher)" || fail=1; echo "$out"
out="$(ts_build_check promotion-ts-builds promotion listCampaigns cancelCampaign updateCampaign)" || fail=1; echo "$out"
out="$(go_build_check identity-builds identity CreateHandoffCode RedeemHandoffCode PeekHandoffCode BecomeSeller)" || fail=1; echo "$out"
out="$(ts_build_check identity-builds identity createHandoffCode redeemHandoffCode peekHandoffCode becomeSeller)" || fail=1; echo "$out"

PROTO_MOD="$ROOT/platform-core/packages/proto"
if (cd "$PROTO_MOD" && $BUF lint) >"$tmp/lint.log" 2>&1; then echo "ok   buf-lint-breaking: buf lint 0 findings"; else echo "FAIL buf-lint-breaking: buf lint: $(head -3 "$tmp/lint.log" | tr '\n' ' ')"; fail=1; fi
base_sha="$(git -C "$ROOT" rev-parse -q --verify "${BREAKING_BASE:-origin/main}" 2>/dev/null)"
gitdir="$(cd "$ROOT" && cd "$(git rev-parse --git-common-dir)" && pwd)"
if [ -n "$base_sha" ] && (cd "$PROTO_MOD" && $BUF breaking --against "$gitdir#ref=$base_sha,subdir=platform-core/packages/proto") >"$tmp/brk.log" 2>&1; then echo "ok   buf-lint-breaking: buf breaking 0 findings"; else echo "FAIL buf-lint-breaking: buf breaking: $(head -3 "$tmp/brk.log" | tr '\n' ' ')"; fail=1; fi

tree_sum() {
  (cd "$1/$2" && find . -type f | LC_ALL=C sort | while read -r f; do shasum "$f"; done | shasum | cut -d' ' -f1)
}
regen_ok=1; rn=0
for dom in promotion identity; do
  for r in $(dom_repos "$dom" go); do
    rn=$((rn+1))
    gen_go "$r" || { echo "FAIL regen-reproducible: regenerate ${r#$ROOT/}"; regen_ok=0; continue; }
    a="$(tree_sum "$r" generated)"
    gen_go "$r" || { echo "FAIL regen-reproducible: second regenerate ${r#$ROOT/}"; regen_ok=0; continue; }
    b="$(tree_sum "$r" generated)"
    [ -n "$a" ] && [ "$a" = "$b" ] || { echo "FAIL regen-reproducible: generated tree differs between runs ${r#$ROOT/}"; regen_ok=0; }
    dirty="$(cd "$ROOT" && git status --porcelain -- "${r#$ROOT/}/proto")"
    [ -z "$dirty" ] || { echo "FAIL regen-reproducible: regeneration modified vendored proto in ${r#$ROOT/}"; regen_ok=0; }
  done
done
[ "$rn" -gt 0 ] || { echo "FAIL regen-reproducible: no Go consumers discovered"; regen_ok=0; }
if [ "$regen_ok" -eq 1 ]; then echo "ok   regen-reproducible: $rn consumer regenerations identical"; else fail=1; fi
exit $fail
