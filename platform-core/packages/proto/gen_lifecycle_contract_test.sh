#!/usr/bin/env bash
# Task 2.2 - scenario "Order lifecycle contract: New contract in every generated language".
# Run AFTER `make -C platform-core proto`. Asserts gen/{go,python,jvm,ts} expose the
# new RPC, enums, messages, fields and PAYMENT_STATUS_CANCELLED. Usage: [GEN_DIR=...] bash <this>
set -u
here="$(cd "$(dirname "$0")" && pwd)"
GEN="${GEN_DIR:-$here/../../gen}"
fail=0
check() { # lang, symbol, path-pattern
  local lang="$1" sym="$2" pat="$3" files=()
  if [ -d "$GEN/$lang" ]; then
    while IFS= read -r f; do files+=("$f"); done < <(find "$GEN/$lang" -type f -path "$pat" 2>/dev/null)
  fi
  # No matching files must fail (never let grep fall back to searching cwd).
  if [ "${#files[@]}" -gt 0 ] && grep -qE -- "$sym" "${files[@]}"; then
    echo "ok   $lang: $sym"
  else
    echo "FAIL $lang: $sym missing in $pat (${#files[@]} files matched)"; fail=1
  fi
}
order_syms="ConfirmReceipt CancelReason CompletedBy OrderPlaced OrderPaymentReminder OrderCompleted ReturnRequested"
for lang_pat in "go:*/order/v1/*.go" "python:*/order/v1/*.py*" "jvm:*/order/v1/*" "ts:*/order/v1/*"; do
  lang="${lang_pat%%:*}"; pat="${lang_pat#*:}"
  for s in $order_syms; do check "$lang" "$s" "$pat"; done
done
# field-level additions (naming differs per language: can_escalate / CanEscalate / canEscalate)
check go     "CanEscalate"  "*/order/v1/order.pb.go"
check go     "ReasonCode"   "*/order/v1/order.pb.go"
check go     "ShipBy"       "*/order/v1/order.pb.go"
check python "can_escalate" "*/order/v1/order_pb2.pyi"
check python "reason_code"  "*/order/v1/order_pb2.pyi"
check ts     "canEscalate"  "*/order/v1/order_pb.*"
check ts     "reasonCode"   "*/order/v1/order_pb.*"
# PAYMENT_STATUS_CANCELLED in every language
check go     "PAYMENT_STATUS_CANCELLED" "*/payment/v1/*.go"
check python "PAYMENT_STATUS_CANCELLED" "*/payment/v1/*.py*"
check jvm    "PAYMENT_STATUS_CANCELLED" "*/payment/v1/*"
check ts     "PAYMENT_STATUS_CANCELLED" "*/payment/v1/*"
exit $fail
