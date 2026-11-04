#!/usr/bin/env bash
set -eu
root="$(cd "$(dirname "$0")/../../.." && pwd)"
f="$root/fe-platform/docs/gaps.md"
if grep -q '^| G8 ' "$f"; then
  echo "FAIL identity-sso-handoff 1.7.1: row G8 still present in fe-platform/docs/gaps.md"
  exit 1
fi
echo "PASS identity-sso-handoff 1.7.1: row G8 removed"
