#!/usr/bin/env bash
# Task 1 - capability "Hardening contract" (change pah-contract). Scenarios:
#   "Hardening contract: ReviewHidden has the agreed fields"
#   "Hardening contract: ReviewHidden carries no user data"
#   "Hardening contract: Change is backward compatible (guard)"       (Red-First-Exempt)
#   "Hardening contract: Removing a ReviewHidden field is rejected"
#   "Hardening contract: Source protos lint clean"
#   "Hardening contract: Catalogue has 18 actions"
#   "Hardening contract: Audit wire format is unchanged (guard)"      (Red-First-Exempt)
#   "Hardening contract: Catalogue count mismatch is rejected"
#   "Hardening contract: Copies match the source"
#   "Hardening contract: Drifted or missing copy is rejected"
#   "Hardening contract: Consumer proto modules lint and build"
#   "Hardening contract: Generated ReviewHidden type exists in consumers"
# Usage: [REPO_ROOT=...] [BASE_REF=<commit>] [PY=python3] PATH=$HOME/go/bin:$PATH bash pah_contract_test.sh
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
PY="${PY:-python3}"
PROTO_REL="platform-core/packages/proto"
SRC="$ROOT/$PROTO_REL"
EV="platform/engagement/v1/events.proto"
AU="platform/audit/v1/audit.proto"
EV_REPOS="team-engagement team-analytics team-order"
AU_REPOS="team-analytics team-audit team-domain team-engagement team-frontend team-gateway team-identity team-order team-payment team-promotion team-verification"
GEN_REPOS="team-engagement team-analytics"

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
fail=0
ok()  { echo "ok   $*"; }
bad() { echo "FAIL $*"; fail=1; }

# --- helpers -----------------------------------------------------------
# msg_fields <proto-module-dir> <proto-path> <Message>: prints "name type number" per field ("" + rc 1 if message missing)
msg_fields() {
  local dir="$1" path="$2" msg="$3"
  (cd "$dir" && buf build --path "$path" --exclude-source-info -o -#format=json 2>/dev/null) | "$PY" -c '
import json,sys
msg=sys.argv[1]
img=json.load(sys.stdin)
for f in img.get("file",[]):
    for m in f.get("messageType",[]):
        if m.get("name")==msg and f.get("package")=="platform.engagement.v1":
            for x in m.get("field",[]):
                print(x["name"],x.get("typeName",x.get("type")),x["number"])
            sys.exit(0)
sys.exit(1)' "$msg"
}

# check_catalogue <audit.proto>: header count must equal distinct listed actions; non-zero + "mismatch" on disagreement
check_catalogue() {
  "$PY" - "$1" <<'PY'
import re,sys
t=open(sys.argv[1]).read().splitlines()
hdr=None; acts=[]
for i,l in enumerate(t):
    m=re.match(r'\s*//\s*Action catalogue \((\d+)\):',l)
    if m:
        hdr=int(m.group(1))
        for l2 in t[i+1:]:
            if not re.match(r'\s*//\s{2,}\S',l2): break
            acts+= [a.strip() for a in l2.split('//',1)[1].split(',') if a.strip()]
        break
if hdr is None: print("mismatch: no 'Action catalogue (N):' header"); sys.exit(1)
if hdr!=18: print("mismatch: header says (%d), want (18)"%hdr); sys.exit(1)
if len(set(acts))!=len(acts) or len(acts)!=hdr:
    print("mismatch: header (%d) but %d listed (%d distinct)"%(hdr,len(acts),len(set(acts)))); sys.exit(1)
if "voucher.admin_create" not in acts: print("mismatch: voucher.admin_create missing"); sys.exit(1)
PY
}

# check_copies <root>: FAIL line with the copy path per drifted/missing copy; non-zero on any
check_copies() {
  local root="$1" rc=0 r
  for r in $EV_REPOS; do
    cmp -s "$root/$PROTO_REL/$EV" "$root/$r/proto/$EV" || { echo "FAIL copies-match: $r/proto/$EV"; rc=1; }
    grep -q 'message ReviewHidden' "$root/$r/proto/$EV" 2>/dev/null || { echo "FAIL content: $r/proto/$EV lacks ReviewHidden"; rc=1; }
  done
  for r in $AU_REPOS; do
    cmp -s "$root/$PROTO_REL/$AU" "$root/$r/proto/$AU" || { echo "FAIL copies-match: $r/proto/$AU"; rc=1; }
    grep -q 'Action catalogue (18)' "$root/$r/proto/$AU" 2>/dev/null || { echo "FAIL content: $r/proto/$AU lacks catalogue (18)"; rc=1; }
  done
  return $rc
}

git_common="$(git -C "$ROOT" rev-parse --git-common-dir 2>/dev/null)"
case "$git_common" in /*) ;; *) git_common="$ROOT/$git_common";; esac
# Baseline = the commit just before implementation: the "test(1): red" commit if it
# exists, else HEAD. (main can lag the slice base, so it is not used by default.)
if [ -z "${BASE_REF:-}" ]; then
  BASE_REF="$(git -C "$ROOT" log --format=%H --grep='^test(1): red' --diff-filter=A -1 -- "$PROTO_REL/pah_contract_test.sh" 2>/dev/null)"
  BASE_REF="${BASE_REF:-HEAD}"
fi
BASE_URL="$git_common#ref=$BASE_REF,subdir=$PROTO_REL"

# --- Scenario: ReviewHidden has the agreed fields -----------------------
got="$(msg_fields "$SRC" "$EV" ReviewHidden)"
want="$(printf 'review_id TYPE_STRING 1\nlisting_id TYPE_STRING 2\nhidden_at .google.protobuf.Timestamp 3')"
if [ "$got" = "$want" ]; then ok "ReviewHidden has the agreed fields"
else bad "ReviewHidden fields: got [$(echo "$got" | tr '\n' ';')] want [$(echo "$want" | tr '\n' ';')]"; fi

# --- Scenario: ReviewHidden carries no user data ------------------------
if [ -z "$got" ]; then bad "no-user-data: ReviewHidden not found"
elif echo "$got" | awk '{print $1}' | grep -Eqx 'user_id|reviewer_id|text|title|body'; then bad "ReviewHidden has a forbidden field"
else ok "ReviewHidden carries no user data"; fi

# --- Scenario: Change is backward compatible (guard) --------------------
if out="$(cd "$SRC" && buf breaking --against "$BASE_URL" 2>&1)"; then ok "buf breaking vs $BASE_REF: 0 findings"
else bad "buf breaking vs $BASE_REF: $(echo "$out" | head -3 | tr '\n' ' ')"; fi

# --- Scenario: Removing a ReviewHidden field is rejected ----------------
# Mutate a synthetic copy of the real module; buf breaking (real source as the
# baseline) must flag the removal / renumbering.
for variant in drop-listing-id renumber-hidden-at; do
  rm -rf "$tmp/syn"; mkdir -p "$tmp/syn"
  cp -R "$SRC/platform" "$SRC/buf.yaml" "$tmp/syn/"
  if "$PY" - "$tmp/syn/$EV" "$variant" <<'PY'
import re,sys
p,v=sys.argv[1:3]
s=open(p).read()
m=re.search(r'message ReviewHidden \{.*?\n\}',s,re.S)
if not m: sys.exit(3)
b=m.group(0)
if v=="drop-listing-id": nb=re.sub(r'[^\n]*\blisting_id\b[^\n]*\n','',b)
else: nb=re.sub(r'(hidden_at\s*=\s*)3',r'\g<1>9',b)
if nb==b: sys.exit(4)
open(p,'w').write(s.replace(b,nb))
PY
  then
    if (cd "$tmp/syn" && buf breaking --against "$SRC" >"$tmp/br.out" 2>&1); then bad "breaking accepted $variant"
    elif [ -s "$tmp/br.out" ]; then ok "breaking rejects $variant"
    else bad "breaking $variant: non-zero but no findings"; fi
  else bad "breaking $variant: could not build synthetic tree (ReviewHidden missing from source?)"; fi
done

# --- Scenario: Source protos lint clean ---------------------------------
# (lint alone is green at base; require ReviewHidden to exist so it covers the new message)
if (cd "$SRC" && buf lint >"$tmp/lint.out" 2>&1) && [ -n "$got" ]; then ok "buf lint clean incl. ReviewHidden"
else bad "buf lint / ReviewHidden: $(head -3 "$tmp/lint.out" | tr '\n' ' ') ${got:+}${got:-(ReviewHidden absent)}"; fi

# --- Scenario: Catalogue has 18 actions ---------------------------------
if out="$(check_catalogue "$SRC/$AU")"; then ok "catalogue has 18 actions incl. voucher.admin_create"
else bad "catalogue: $out"; fi

# --- Scenario: Audit wire format is unchanged (guard) -------------------
cur="$(cd "$SRC" && buf build --path "$AU" --exclude-source-info -o -#format=json 2>&1)"
base="$(buf build "$BASE_URL" --path "$AU" --exclude-source-info -o -#format=json 2>&1)"
if [ -n "$cur" ] && [ "$cur" = "$base" ]; then ok "audit.proto descriptor identical to $BASE_REF"
else bad "audit.proto descriptor differs from $BASE_REF (or build failed)"; fi

# --- Scenario: Catalogue count mismatch is rejected ---------------------
# synthetic file: header (18) over a 17-action list
cat > "$tmp/aud17.proto" <<'P'
// Action catalogue (18):
//   review.hide, kyc.review, dispute.resolve, listing.admin_update,
//   listing.admin_delete, order.force_fail, order.admin_status_update,
//   return.admin_status_update, shipment.admin_create, payment.admin_refund,
//   voucher.platform_create, flash_sale.admin_create, ad_campaign.admin_create,
//   listing.moderate, user.set_status, report.resolve, return.dispute_refund
message WriteAuditEventRequest {}
P
if out="$(check_catalogue "$tmp/aud17.proto")"; then bad "catalogue (18) over 17 actions accepted"
elif echo "$out" | grep -qi mismatch; then ok "catalogue count mismatch rejected"
else bad "catalogue mismatch rejected without naming it: $out"; fi

# --- Scenario: Copies match the source ----------------------------------
if out="$(check_copies "$ROOT")"; then ok "copies match (14)"; else echo "$out"; bad "copies match"; fi

# --- Scenario: Drifted or missing copy is rejected ----------------------
mk() {
  rm -rf "$tmp/t"; mkdir -p "$tmp/t/$PROTO_REL/platform/engagement/v1" "$tmp/t/$PROTO_REL/platform/audit/v1"
  printf 'message ReviewHidden\n' > "$tmp/t/$PROTO_REL/$EV"; printf 'Action catalogue (18)\n' > "$tmp/t/$PROTO_REL/$AU"
  local r
  for r in $EV_REPOS; do mkdir -p "$tmp/t/$r/proto/platform/engagement/v1"; cp "$tmp/t/$PROTO_REL/$EV" "$tmp/t/$r/proto/$EV"; done
  for r in $AU_REPOS; do mkdir -p "$tmp/t/$r/proto/platform/audit/v1"; cp "$tmp/t/$PROTO_REL/$AU" "$tmp/t/$r/proto/$AU"; done
}
mk
check_copies "$tmp/t" >/dev/null && ok "faithful tree accepted" || bad "faithful synthetic tree rejected"
mk; printf 'x' >> "$tmp/t/team-order/proto/$EV"
if out="$(check_copies "$tmp/t")"; then bad "one-byte drift accepted"
elif echo "$out" | grep -q "team-order/proto/$EV"; then ok "one-byte drift rejected, path named"
else bad "drift rejected without path: $out"; fi
mk; rm "$tmp/t/team-gateway/proto/$AU"
if out="$(check_copies "$tmp/t")"; then bad "missing copy accepted"
elif echo "$out" | grep -q "team-gateway/proto/$AU"; then ok "missing copy rejected, path named"
else bad "missing copy rejected without path: $out"; fi

# --- Scenario: Consumer proto modules lint and build --------------------
for r in $GEN_REPOS; do
  d="$ROOT/$r/proto"
  if (cd "$d" && buf lint >"$tmp/l.out" 2>&1 && buf build >>"$tmp/l.out" 2>&1) \
     && msg_fields "$d" "$EV" ReviewHidden >/dev/null; then ok "consumer module $r lints, builds, has ReviewHidden"
  else bad "consumer module $r: $(head -3 "$tmp/l.out" | tr '\n' ' ') (or ReviewHidden missing)"; fi
done

# --- Scenario: Generated ReviewHidden type exists in consumers ----------
# Uses local plugins when present (BSR rate limits), the repo template otherwise.
for r in $GEN_REPOS; do
  d="$ROOT/$r"; tpl="$d/buf.gen.yaml"
  if command -v protoc-gen-go >/dev/null 2>&1 && command -v protoc-gen-go-grpc >/dev/null 2>&1; then
    tpl="$tmp/$r.gen.yaml"
    sed -e 's#remote: buf.build/protocolbuffers/go#local: protoc-gen-go#' \
        -e 's#remote: buf.build/grpc/go#local: protoc-gen-go-grpc#' "$d/buf.gen.yaml" > "$tpl"
  fi
  if ! (cd "$d" && buf generate --template "$tpl" >"$tmp/g.out" 2>&1); then bad "buf generate $r: $(head -3 "$tmp/g.out" | tr '\n' ' ')"; continue; fi
  gf="$d/generated/$EV"; gf="${gf%.proto}.pb.go"
  miss=""
  for pat in 'type ReviewHidden struct' 'ReviewHidden) GetReviewId()' 'ReviewHidden) GetListingId()' 'ReviewHidden) GetHiddenAt()'; do
    grep -q "$pat" "$gf" 2>/dev/null || miss="$miss [$pat]"
  done
  if [ -n "$miss" ]; then bad "generated $r lacks:$miss"; continue; fi
  if (cd "$d" && go build ./... >"$tmp/b.out" 2>&1); then ok "generated ReviewHidden in $r, go build ok"
  else bad "go build $r: $(head -3 "$tmp/b.out" | tr '\n' ' ')"; fi
done

exit $fail
