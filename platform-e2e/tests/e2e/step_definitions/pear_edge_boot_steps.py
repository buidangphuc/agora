"""Boot guards for port-edge-authz-residuals (deploy-runtime), run as black boxes.

The real image is started with `docker run` on the stack network with the compose
environment plus the offending setting; the assertions are the exit code and the log. If a
guard is missing the process would keep running, so the run is bounded and the container is
removed either way.
"""

from __future__ import annotations

import subprocess
import tempfile
import uuid

from pytest_bdd import then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

RUN_TIMEOUT_S = 60


def _run(
    world: World, image: str, env: dict[str, str], label: str
) -> subprocess.CompletedProcess[str]:
    name = f"e2e-{label}-boot-{uuid.uuid4().hex[:8]}"
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    cmd = ["docker", "run", "--rm", "--name", name, "--network", pe.stack_network()]
    for key, value in env.items():
        cmd += ["-e", f"{key}={value}"]
    cmd.append(image)
    try:
        return subprocess.run(cmd, capture_output=True, text=True, timeout=RUN_TIMEOUT_S)
    except subprocess.TimeoutExpired as exc:
        pe.docker("rm", "-f", name, check=False)
        raise AssertionError(
            f"{image} kept running instead of refusing to boot: {exc.stdout}"
        ) from exc


def _assert_refused(world: World, setting: str) -> None:
    proc: subprocess.CompletedProcess[str] = world.state.extra["pe_boot"]
    log = proc.stdout + proc.stderr
    assert proc.returncode != 0, f"exit 0, booted with {setting}:\n{log}"
    assert setting in log, log


@when("the team-gateway image is started with ENV=production and EDGE_REFLECTION_ENABLED=true")
def start_gateway(world: World) -> None:
    env = pe.compose_env("team-gateway")
    env.update(ENV="production", EDGE_REFLECTION_ENABLED="true")
    world.state.extra["pe_boot"] = _run(world, pe.gateway_boot_image(), env, "gateway")


@then("the gateway process exits non-zero and its log names EDGE_REFLECTION_ENABLED")
def gateway_refused(world: World) -> None:
    _assert_refused(world, "EDGE_REFLECTION_ENABLED")


def _own_signing_key() -> str:
    with tempfile.TemporaryDirectory() as tmp:
        key = f"{tmp}/k.pem"
        subprocess.run(
            [
                "openssl",
                "genpkey",
                "-algorithm",
                "RSA",
                "-pkeyopt",
                "rsa_keygen_bits:2048",
                "-out",
                key,
            ],
            check=True,
            capture_output=True,
        )
        return open(key).read()


@when(
    "the team-identity image is started with ENV=staging, its own non-development signing key, "
    "and PASSWORD_RESET_EXPOSE_TOKEN=true"
)
def start_identity(world: World) -> None:
    env = pe.compose_env("team-identity")
    env.update(
        ENV="staging",
        JWT_PRIVATE_KEY=_own_signing_key(),
        JWT_KID=f"e2e-staging-{uuid.uuid4().hex[:8]}",
        PASSWORD_RESET_EXPOSE_TOKEN="true",
        SEED_ADMIN_ENABLED="false",
    )
    world.state.extra["pe_boot"] = _run(world, pe.identity_boot_image(), env, "identity")


@then("the identity process exits non-zero and its log names PASSWORD_RESET_EXPOSE_TOKEN")
def identity_refused(world: World) -> None:
    _assert_refused(world, "PASSWORD_RESET_EXPOSE_TOKEN")
