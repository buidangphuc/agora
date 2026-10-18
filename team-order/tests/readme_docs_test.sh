#!/usr/bin/env bash
# task 11 (order-receipt-completion): README documents timers, env vars, events.
set -u
f="$(dirname "$0")/../README.md"; rc=0
for k in AUTO_COMPLETE_AFTER OrderCompleted; do
  grep -q "$k" "$f" || { echo "FAIL: README missing $k"; rc=1; }
done
exit $rc
