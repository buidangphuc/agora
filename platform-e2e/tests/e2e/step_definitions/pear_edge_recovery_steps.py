"""Upstream recovery after a container recreate (port-edge-authz-residuals, destructive).

team-payment is recreated through the stack's compose wrapper (DC_WRAPPER) so it comes back
on a new address; the port-ready moment is measured from a probe on the stack network, and
the 5 second bound is counted from there, through the public gateway.
"""

from __future__ import annotations

import os
import subprocess
import time

import httpx
from pytest_bdd import given, then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

WALLET = "/platform.payment.v1.PaymentService/GetWalletBalance"
SERVICE = "team-payment"
HOST, PORT = "team-payment-svc", 50056
WITHIN_S = 5.0
PROBE = (
    "import socket,sys,time\n"
    "end=time.time()+90\n"
    "while time.time()<end:\n"
    "    try:\n"
    f"        socket.create_connection(('{HOST}',{PORT}),1).close(); sys.exit(0)\n"
    "    except OSError:\n"
    "        time.sleep(0.1)\n"
    "sys.exit(1)\n"
)


def _x(world: World) -> dict:
    return world.state.extra


def _wallet_ok(token: str) -> bool:
    try:
        return pe.post_json(WALLET, {}, token, timeout=3).status_code == 200
    except httpx.HTTPError:
        return False


@given("a logged-in seller whose wallet balance can be read through the gateway")
def seller_with_wallet(world: World) -> None:
    token = pe.register(world, "seller")
    assert _wallet_ok(token), "GetWalletBalance does not succeed before the recreate"
    _x(world)["pe_token"] = token

    def settle() -> None:
        pe.wait_healthy(pe.payment_container(), 120)
        deadline = time.monotonic() + 120
        streak = 0
        while time.monotonic() < deadline and streak < 3:
            streak = streak + 1 if _wallet_ok(token) else 0
            time.sleep(0.5)
        assert streak >= 3, "team-payment wallet reads did not become stable after the recreate"

    world.add_cleanup(settle)


@when("the team-payment container is recreated and its gRPC port answers")
def recreate_payment(world: World) -> None:
    wrapper = os.getenv("DC_WRAPPER", pe.DC_WRAPPER_DEFAULT)
    subprocess.run(
        [wrapper, "up", "-d", "--force-recreate", "--no-deps", SERVICE],
        check=True,
        capture_output=True,
        text=True,
        timeout=180,
    )
    probe_image = os.getenv("PROBE_IMAGE", "python:3.12-alpine")
    subprocess.run(
        [
            "docker",
            "run",
            "--rm",
            "--network",
            pe.stack_network(),
            probe_image,
            "python",
            "-c",
            PROBE,
        ],
        check=True,
        capture_output=True,
        text=True,
        timeout=120,
    )
    _x(world)["pe_port_ready"] = time.monotonic()


@then("within 5 seconds the seller's GetWalletBalance through the gateway succeeds")
def wallet_within_5s(world: World) -> None:
    x = _x(world)
    deadline = x["pe_port_ready"] + WITHIN_S
    while time.monotonic() < deadline:
        if _wallet_ok(x["pe_token"]):
            return
        time.sleep(0.2)
    raise AssertionError(
        f"GetWalletBalance still failing {WITHIN_S}s after team-payment's port answered"
    )


@then("it keeps succeeding on 3 consecutive calls")
def wallet_keeps_succeeding(world: World) -> None:
    token = _x(world)["pe_token"]
    results = []
    for _ in range(3):
        results.append(_wallet_ok(token))
        time.sleep(0.3)
    assert all(results), f"consecutive GetWalletBalance results: {results}"
