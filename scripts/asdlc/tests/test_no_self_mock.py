import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).parent
SCRIPT = HERE.parent / "no_self_mock.py"
FX = HERE / "fixtures" / "no_self_mock"


def run(*paths):
    return subprocess.run([sys.executable, str(SCRIPT), *map(str, paths)], capture_output=True, text=True)


def rules(r):
    return [l.split()[1] for l in r.stdout.splitlines()]


def test_go_bad():
    r = run(FX / "bad_integration_test.go")
    assert r.returncode == 1
    assert {"go-gomock", "go-fake-grpc-client"} <= set(rules(r))


def test_go_allowlisted():
    r = run(FX / "allowed_integration_test.go")
    assert r.returncode == 0, r.stdout


def test_py_bad():
    r = run(FX / "test_bad.py")
    assert r.returncode == 1
    assert rules(r).count("py-mock-own-package") == 4
    assert "py-route-own-gateway" in rules(r)


def test_py_allowlisted():
    r = run(FX / "test_allowed.py")
    assert r.returncode == 0, r.stdout


def test_ts_bad():
    r = run(FX / "bad.spec.ts")
    assert r.returncode == 1
    assert rules(r).count("ts-route-own-gateway") == 2


def test_ts_allowlisted():
    r = run(FX / "allowed.spec.ts")
    assert r.returncode == 0, r.stdout
