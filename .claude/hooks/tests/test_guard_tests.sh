#!/usr/bin/env bash
# Tests for guard-tests.sh: pipe sample PreToolUse payloads, check exit codes.
set -u
here="$(cd "$(dirname "$0")" && pwd)"
hook="$here/../guard-tests.sh"
fail=0

check() { # name expected agent tool input_json
  local name="$1" want="$2" agent="$3" tool="$4" input="$5" payload rc
  if [ -n "$agent" ]; then
    payload="$(jq -nc --arg a "$agent" --arg t "$tool" --argjson i "$input" '{agent_type:$a,tool_name:$t,tool_input:$i}')"
  else
    payload="$(jq -nc --arg t "$tool" --argjson i "$input" '{tool_name:$t,tool_input:$i}')"
  fi
  printf '%s' "$payload" | bash "$hook" >/dev/null 2>&1; rc=$?
  if [ "$rc" -eq "$want" ]; then echo "ok   $name"; else echo "FAIL $name (want $want got $rc)"; fail=1; fi
}

check "implementer edits foo_test.go"        2 implementer Edit  '{"file_path":"team-x/foo_test.go"}'
check "implementer edits foo.go"             0 implementer Edit  '{"file_path":"team-x/foo.go"}'
check "test-author edits foo_test.go"        0 test-author Edit  '{"file_path":"team-x/foo_test.go"}'
check "no agent_type edits foo_test.go"      0 ""          Edit  '{"file_path":"team-x/foo_test.go"}'
check "implementer writes a.spec.ts"         2 implementer Write '{"file_path":"web/a.spec.ts"}'
check "implementer writes a.test.ts"         2 implementer MultiEdit '{"file_path":"web/a.test.ts"}'
check "implementer writes test_a.py"         2 implementer Write '{"file_path":"svc/test_a.py"}'
check "implementer writes a_test.py"         2 implementer Write '{"file_path":"svc/a_test.py"}'
check "implementer writes tests/dir file"    2 implementer Write '{"file_path":"svc/tests/conftest.py"}'
check "implementer writes .feature"          2 implementer Write '{"file_path":"x/login.feature"}'
check "implementer writes platform-e2e"      2 implementer Write '{"file_path":"platform-e2e/steps/a.ts"}'
check "implementer notebook in tests"        2 implementer NotebookEdit '{"notebook_path":"svc/tests/n.ipynb"}'
check "implementer bash > tests path"        2 implementer Bash  '{"command":"echo x > a/tests/b.py"}'
check "implementer bash >> _test.go"         2 implementer Bash  '{"command":"echo x >> pkg/a_test.go"}'
check "implementer bash tee"                 2 implementer Bash  '{"command":"echo x | tee pkg/a_test.go"}'
check "implementer bash sed -i"              2 implementer Bash  '{"command":"sed -i s/a/b/ pkg/a_test.go"}'
check "implementer bash rm"                  2 implementer Bash  '{"command":"rm -f svc/tests/old.py"}'
check "implementer bash mv"                  2 implementer Bash  '{"command":"mv a.go pkg/a_test.go"}'
check "implementer bash cp"                  2 implementer Bash  '{"command":"cp x.feature platform-e2e/f.feature"}'
check "implementer bash go test (read only)" 0 implementer Bash  '{"command":"go test ./... 2>&1 | tail -20"}'
check "implementer bash cat test file"       0 implementer Bash  '{"command":"cat pkg/a_test.go"}'
check "implementer bash write prod file"     0 implementer Bash  '{"command":"echo x > pkg/a.go"}'
check "test-author bash > tests path"        0 test-author Bash  '{"command":"echo x > a/tests/b.py"}'
check "no agent_type bash > tests path"      0 ""          Bash  '{"command":"echo x > a/tests/b.py"}'

[ "$fail" -eq 0 ] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
