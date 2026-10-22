#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
core="$(cd "$here/.." && pwd)"
tpl="$core/templates/_service-go"
sdk="$core/packages/go-sdk"
sdk_path="github.com/buidangphuc/agora/platform-core/packages/go-sdk"
old_mod="github.com/your-org/team-service"
new_mod="example.com/scaffold-smoke"

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
fail() { echo "FAIL: A service scaffolded from the template builds without platform copies: $*" >&2; exit 1; }

cp -R "$tpl/." "$scratch/svc"
cd "$scratch/svc"

grep -q "go-sdk v1\." go.mod || fail "template go.mod does not require the SDK at a tag"
grep -q '^replace' go.mod && fail "template go.mod contains a replace"

grep -rl "$old_mod" . 2>/dev/null | while read -r f; do
  sed "s#$old_mod#$new_mod#g" "$f" >"$f.tmp" && mv "$f.tmp" "$f"
done

for p in internal/interceptor internal/grpcserver third_party; do
  [ -e "$p" ] && fail "$p exists in the scaffolded service"
done

printf '\nreplace %s => %s\n' "$sdk_path" "$sdk" >>go.mod

export GOFLAGS=-mod=mod
go mod tidy >"$scratch/tidy.log" 2>&1 || { cat "$scratch/tidy.log" >&2; fail "go mod tidy"; }
go build ./... >"$scratch/build.log" 2>&1 || { cat "$scratch/build.log" >&2; fail "go build ./..."; }
go vet ./... >"$scratch/vet.log" 2>&1 || { cat "$scratch/vet.log" >&2; fail "go vet ./..."; }

for p in internal/interceptor internal/grpcserver third_party; do
  [ -e "$p" ] && fail "$p exists after build"
done

echo "PASS: A service scaffolded from the template builds without platform copies"
