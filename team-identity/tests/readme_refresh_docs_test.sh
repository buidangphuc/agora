#!/usr/bin/env bash
# task 1.4 (Session client context: rotation records IP via repository / empty client keeps stored values)
# README must state that refresh rotation updates session ip, device and last_seen.
set -u
readme="$(dirname "$0")/../README.md"
fail=0
# lines mentioning refresh that also name ip, device and last_seen (case-insens), excluding none
if ! grep -i -E 'refresh' "$readme" | grep -i -E 'last_seen' | grep -i -E '\bip\b' | grep -i -E 'device' >/dev/null; then
  echo "FAIL 1.4: README has no line saying refresh updates session ip, device and last_seen"; fail=1
fi
# stale claim must be gone: last_seen set only at creation and refresh with nothing about client info
if grep -i -E 'empty|keeps?|unchanged|retain' "$readme" | grep -i -E 'refresh' | grep -i -E 'client|ip|device' >/dev/null; then :; else
  echo "FAIL 1.4: README does not say an empty client context keeps stored ip/device"; fail=1
fi
exit $fail
