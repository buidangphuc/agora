#!/usr/bin/env bash
# Change discovery-seo-contract, task 1.7. Scenarios (capability "Discovery SEO contract"):
#   "Discovery SEO contract: Copies match the source"
#   "Discovery SEO contract: Drifted or missing copy is rejected"
#   "Discovery SEO contract: Renumbering is rejected"
#   "Discovery SEO contract: Consumer protos build"
#   "Discovery SEO contract: buf lint and breaking pass"
# Usage: [REPO_ROOT=...] [BASE_REF=origin/main] bash platform-core/packages/proto/discovery_seo_consumer_copies_test.sh
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"
SRC_MOD="$ROOT/platform-core/packages/proto"
BASE_REF="${BASE_REF:-origin/main}"

SEARCH="platform/search/v1/search.proto"
ORDER="platform/order/v1/order.proto"
EVENTS="platform/engagement/v1/events.proto"
RECO="platform/recommendation/v1/recommendation.proto"
ANALYTICS="platform/analytics/v1/analytics.proto"

SEARCH_REPOS="team-ai team-analytics team-chat team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-identity team-notification team-order team-payment team-search"
ORDER_REPOS="team-analytics team-chat team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-notification team-order team-payment team-promotion team-referral team-search"
EVENTS_REPOS="team-analytics team-engagement team-order"
RECO_REPOS="team-ai team-analytics team-frontend team-frontend-admin team-frontend-seller team-gateway"
ANALYTICS_REPOS="team-analytics team-frontend team-frontend-admin team-frontend-seller team-gateway"

pairs() {
  local r
  for r in $SEARCH_REPOS; do echo "$SEARCH $r"; done
  for r in $ORDER_REPOS; do echo "$ORDER $r"; done
  for r in $EVENTS_REPOS; do echo "$EVENTS $r"; done
  for r in $RECO_REPOS; do echo "$RECO $r"; done
  for r in $ANALYTICS_REPOS; do echo "$ANALYTICS $r"; done
}
NPAIRS="$(pairs | wc -l | tr -d ' ')"
UNION="$(pairs | awk '{print $2}' | sort -u)"

check_copies() {
  local root="$1" rc=0 rel r
  while read -r rel r; do
    cmp -s "$root/$SRC_REL/${rel#platform/}" "$root/$r/proto/$rel" \
      || { echo "FAIL copies-match: $r/proto/$rel"; rc=1; }
  done < <(pairs)
  return $rc
}

fail=0
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# Scenario: Copies match the source
if [ "$NPAIRS" != 42 ]; then echo "FAIL pair table has $NPAIRS entries, expected 42 (14+14+3+6+5)"; fail=1; fi
if out="$(check_copies "$ROOT")"; then echo "ok   copies match ($NPAIRS)"; else echo "$out"; fail=1; fi

# Scenario: Drifted or missing copy is rejected (the check itself, on a synthetic tree)
mk() {
  rm -rf "$tmp/t"
  local rel r
  while read -r rel r; do
    mkdir -p "$tmp/t/$SRC_REL/$(dirname "${rel#platform/}")" "$tmp/t/$r/proto/$(dirname "$rel")"
    [ -f "$tmp/t/$SRC_REL/${rel#platform/}" ] || printf 'source %s\n' "$rel" > "$tmp/t/$SRC_REL/${rel#platform/}"
    cp "$tmp/t/$SRC_REL/${rel#platform/}" "$tmp/t/$r/proto/$rel"
  done < <(pairs)
}
mk
check_copies "$tmp/t" >/dev/null && echo "ok   faithful tree accepted" || { echo "FAIL faithful synthetic tree rejected"; fail=1; }
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"; rm "$tmp/t/team-ai/proto/$RECO"
if out="$(check_copies "$tmp/t")"; then echo "FAIL drift and missing copy accepted"; fail=1
elif echo "$out" | grep -q "team-order/proto/$ORDER" && echo "$out" | grep -q "team-ai/proto/$RECO"; then echo "ok   drifted and missing copy rejected and named"
else echo "FAIL drift/missing rejected without naming both: $out"; fail=1; fi

# Scenario: Renumbering is rejected (scratch copy of the module, SearchHit.stock renumbered)
rm -rf "$tmp/mod"; mkdir -p "$tmp/mod"; cp -R "$SRC_MOD/buf.yaml" "$SRC_MOD/platform" "$tmp/mod/"
if sed -i.bak -E '/^message SearchHit /,/^}/ s/(stock = )3;/\18;/' "$tmp/mod/platform/search/v1/search.proto" \
   && ! cmp -s "$tmp/mod/platform/search/v1/search.proto" "$SRC_MOD/platform/search/v1/search.proto"; then
  if (cd "$tmp/mod" && buf breaking --against "$SRC_MOD") >"$tmp/renum.out" 2>&1; then echo "FAIL renumbering accepted by buf breaking"; fail=1
  elif grep -q "SearchHit" "$tmp/renum.out"; then echo "ok   renumbering rejected and names SearchHit"
  else echo "FAIL renumbering rejected without naming SearchHit: $(head -3 "$tmp/renum.out" | tr '\n' ' ')"; fail=1; fi
else echo "FAIL renumbering: could not mutate SearchHit.stock in scratch copy"; fail=1; fi

# Scenario: Consumer protos build (every repo in the union, no sampling)
built=""
for r in $UNION; do
  if [ ! -d "$ROOT/$r/proto" ]; then echo "FAIL buf build $r: proto dir missing"; fail=1; continue; fi
  if (cd "$ROOT/$r/proto" && buf build >"$tmp/build.err" 2>&1); then echo "ok   buf build $r"; built="$built $r"
  else echo "FAIL buf build $r: $(head -3 "$tmp/build.err" | tr '\n' ' ')"; fail=1; fi
done
missing=""
for r in $UNION; do case " $built " in *" $r "*) ;; *) missing="$missing $r";; esac; done
if [ -n "$missing" ]; then echo "FAIL build list incomplete, missing:$missing"; fail=1
else echo "ok   built all $(echo "$UNION" | wc -l | tr -d ' ') repos in the union"; fi

# Scenario: buf lint and breaking pass
if (cd "$SRC_MOD" && buf lint) >"$tmp/lint.out" 2>&1; then echo "ok   buf lint"; else echo "FAIL buf lint: $(head -3 "$tmp/lint.out" | tr '\n' ' ')"; fail=1; fi
if (cd "$SRC_MOD" && buf breaking --against "$ROOT/.git#ref=$BASE_REF,subdir=platform-core/packages/proto") >"$tmp/brk.out" 2>&1; then echo "ok   buf breaking vs $BASE_REF"
else echo "FAIL buf breaking vs $BASE_REF: $(head -3 "$tmp/brk.out" | tr '\n' ' ')"; fail=1; fi
exit $fail
