#!/usr/bin/env bash
# Builds throwaway git repos with a tiny Go module and checks red_first.sh verdicts.
set -uo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
RF="$HERE/../red_first.sh"
FX="$HERE/fixtures/red_first"
T=$(mktemp -d "${TMPDIR:-/tmp}/rf-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
FAILS=0
ok()   { echo "PASS $1"; }
bad()  { echo "FAIL $1"; FAILS=$((FAILS+1)); }

new_repo() { # new_repo <dir>: main has go.mod + Add
  rm -rf "$1"; mkdir -p "$1"; cd "$1" || exit 1
  git init -q -b main . && git config user.email t@t && git config user.name t
  cp "$FX/go.mod" go.mod; cp "$FX/calc_base.go" calc.go
  git add -A && git commit -q -m base
  git checkout -q -b feat
}
commit() { git add -A && git commit -q -m "$1"; }

# (a) test committed before impl, plus e2e feature before impl -> pass
new_repo "$T/a"
cp "$FX/calc_new_test.go" calc_test.go; mkdir -p platform-e2e/features; cp "$FX/mul.feature" platform-e2e/features/mul.feature
commit "test first"
cp "$FX/calc_impl.go" calc.go; commit "impl"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 0 ] && echo "$OUT" | grep -q "red@base=yes" && echo "$OUT" | grep -q "green@head=yes" && echo "$OUT" | grep -q "first-e2e=#1" && ok "(a) test-before-impl passes" || bad "(a) expected exit 0 got $RC"
"$RF" --dry-run >"$T/dry.txt" 2>&1; sed 's/^/    /' "$T/dry.txt"
grep -q "WOULD RUN go: calc_test.go" "$T/dry.txt" && ok "(a) dry-run lists" || bad "(a) dry-run"

# (b) test that already passes at base -> caught
new_repo "$T/b"
cp "$FX/calc_green_test.go" calc_test.go; commit "late test"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 1 ] && echo "$OUT" | grep -q "green at base" && ok "(b) green-at-base caught" || bad "(b) expected exit 1 got $RC"

# (c) e2e feature committed AFTER impl -> caught
new_repo "$T/c"
cp "$FX/calc_impl.go" calc.go; commit "impl"
mkdir -p platform-e2e/features; cp "$FX/mul.feature" platform-e2e/features/mul.feature; commit "feature late"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 1 ] && echo "$OUT" | grep -q "not after first e2e" && ok "(c) e2e-after-impl caught" || bad "(c) expected exit 1 got $RC"

# (d) test-support file (testdata/) must be copied to base too: test is green at base only with it
new_repo "$T/d"
mkdir -p testdata; echo 5 > testdata/want.txt
cat > calc_data_test.go <<'GO'
package calc

import (
	"os"
	"strings"
	"testing"
)

func TestAddFromData(t *testing.T) {
	b, err := os.ReadFile("testdata/want.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "5" || Add(2, 3) != 5 {
		t.Fatal("bad")
	}
}
GO
commit "test with testdata"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 1 ] && echo "$OUT" | grep -q "green at base" && ok "(d) support files copied to base" || bad "(d) expected green-at-base (exit 1) got $RC"

# (e) uncommitted code/test changes -> refuse (unless --allow-dirty)
new_repo "$T/e"
cp "$FX/calc_new_test.go" calc_test.go; commit "test first"
cp "$FX/calc_impl.go" calc.go
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 1 ] && echo "$OUT" | grep -q "uncommitted" && ok "(e) dirty tree refused" || bad "(e) expected exit 1 got $RC"
"$RF" --allow-dirty 2>&1 | grep -q "uncommitted" && bad "(e) --allow-dirty" || ok "(e) --allow-dirty skips the check"
echo "// scratch" > untracked_test.go
"$RF" >/dev/null 2>&1; [ $? -eq 1 ] && ok "(e) untracked test file refused" || bad "(e) untracked"

# (f) impl changed, no tests -> fail; --allow-empty -> 0
new_repo "$T/f"
cp "$FX/calc_impl.go" calc.go; commit "impl only"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 1 ] && echo "$OUT" | grep -q "no tests changed" && ok "(f) impl without tests fails" || bad "(f) expected exit 1 got $RC"
"$RF" --allow-empty >/dev/null 2>&1; [ $? -eq 0 ] && ok "(f) --allow-empty ok" || bad "(f) --allow-empty"

# (g) --only python skips go tests
new_repo "$T/g"
cp "$FX/calc_green_test.go" calc_test.go; commit "late go test"
OUT=$("$RF" --only python 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 0 ] && echo "$OUT" | grep -q "SKIP go tests" && ok "(g) --only skips go" || bad "(g) expected exit 0 got $RC"

# (h) Red-First-Exempt trailer: green-at-base test passes as EXEMPT; a trailer without reason is ignored
new_repo "$T/h"
cp "$FX/calc_green_test.go" calc_test.go; git add -A && git commit -q -m "adapt test to lib bump" -m "Red-First-Exempt: calc_*.go -- dependency bump, no new behaviour"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 0 ] && echo "$OUT" | grep -q "EXEMPT (dependency bump" && ok "(h) exempt trailer honoured" || bad "(h) expected exit 0 got $RC"
new_repo "$T/h2"
cp "$FX/calc_green_test.go" calc_test.go; git add -A && git commit -q -m "late test" -m "Red-First-Exempt: calc_test.go"
OUT=$("$RF" 2>&1); RC=$?
[ $RC -eq 1 ] && echo "$OUT" | grep -q "green at base" && ok "(h) exempt without reason ignored" || bad "(h2) expected exit 1 got $RC"

# (i) gitignored generated proto code: a module with buf.gen.yaml is regenerated in both worktrees
new_repo "$T/i"
mkdir -p "$T/bin"
cat > "$T/bin/buf" <<'SH'
#!/usr/bin/env bash
# fake buf: `buf generate --template <t>` writes the generated package
[ "$1" = generate ] || exit 2
mkdir -p generated && printf 'package generated\n\nconst Factor = 1\n' > generated/gen.go
SH
chmod +x "$T/bin/buf"
printf 'generated/\n' > .gitignore
printf 'version: v2\n' > buf.gen.yaml
commit "buf config"
cat > calc_test.go <<'GO'
package calc

import (
	"testing"

	"example.com/calc/generated"
)

func TestMulGenerated(t *testing.T) {
	if Mul(2, 3)*generated.Factor != 6 {
		t.Fatal("mul")
	}
}
GO
commit "test first"
cp "$FX/calc_impl.go" calc.go; commit "impl"
OUT=$(PATH="$T/bin:$PATH" "$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 0 ] && echo "$OUT" | grep -q "green@head=yes" && ok "(i) generated code built in worktrees" || bad "(i) expected exit 0 got $RC"

# (j) run metrics (metrics/**) are not implementation: no "impl without tests" failure
new_repo "$T/j"
mkdir -p metrics/runs; echo '{"event":"run_done"}' > metrics/runs/x.jsonl; commit "run metrics"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 0 ] && echo "$OUT" | grep -q "impl=0" && ok "(j) metrics are non-impl" || bad "(j) expected exit 0 got $RC"

# (k) an exempted helper-only _test.go (no Test funcs, e.g. a vendored SDK fake) is skipped, not failed
new_repo "$T/k"
printf 'package calc\n\nfunc helperOnly() int { return 1 }\n' > helpers_test.go
git add -A && git commit -q -m "vendor helper" -m "Red-First-Exempt: helpers_test.go -- vendored helper, no tests"
OUT=$("$RF" 2>&1); RC=$?
echo "$OUT" | sed 's/^/    /'
[ $RC -eq 0 ] && echo "$OUT" | grep -q "SKIP (no Test funcs)" && ok "(k) exempted helper-only test file skipped" || bad "(k) expected exit 0 got $RC"

[ $FAILS -eq 0 ] && echo "ALL OK" || echo "$FAILS FAILED"
exit $FAILS
