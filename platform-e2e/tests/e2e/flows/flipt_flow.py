"""Flipt feature-flag seeding flow (wire-openfeature).

Toggles the boolean `checkout-enabled` kill-switch directly through Flipt's
REST/HTTP API so a scenario can drive OFF/ON per its Given/When. This is the e2e
equivalent of an operator flipping the flag in the Flipt UI — the running
services pick up the new state within seconds via their streamed in-memory
snapshot, with no redeploy.

Flipt is a stack dependency reached at `Settings.flipt_url` (:8080 REST). httpx
is already a repo dependency, so this module imports at top level. The exact
Flipt REST shape (namespace/flag/create) is confirmed at apply time against the
`add-flipt-infra` deployment; calls are best-effort/idempotent.
"""

from __future__ import annotations

import os
import subprocess
import sys
import time
from pathlib import Path
from typing import Any

import httpx

from config.settings import get_settings

FLAG_KEY = "checkout-enabled"
NAMESPACE = "default"


def _client() -> httpx.Client:
    return httpx.Client(base_url=get_settings().flipt_url.rstrip("/"), timeout=15.0)


def set_checkout_flag(enabled: bool) -> bool:
    """Set `checkout-enabled` to `enabled` in Flipt. Returns the applied state.

    Ensures the boolean flag exists (creating it on first run), then updates its
    `enabled` state. Idempotent: when the flag already has the wanted value nothing
    is written. Flipt keeps its flags in SQLite, so every UpdateFlag takes the write
    lock and stalls the Boolean evaluations of concurrent CreateOrders (measured 2-7 s
    on both sides); the ON scenarios run in parallel with all other checkouts and must
    not cause that when the flag is already ON.
    """
    base = f"/api/v1/namespaces/{NAMESPACE}/flags"
    payload = {
        "key": FLAG_KEY,
        "name": "Checkout enabled",
        "description": "Emergency kill-switch for the checkout / place-order path.",
        "type": "BOOLEAN_FLAG_TYPE",
        "enabled": enabled,
    }
    with _client() as client:
        current = client.get(f"{base}/{FLAG_KEY}")
        if current.status_code == httpx.codes.OK and current.json().get("enabled") is enabled:
            return enabled
        # Update in place; create it if this env has never seen the flag.
        resp = client.put(f"{base}/{FLAG_KEY}", json=payload)
        if resp.status_code == httpx.codes.NOT_FOUND:
            client.post(base, json=payload)
    return enabled


EXPERIMENT_FLAG = "recs-home-feed-ranker"
EXPERIMENT_SEGMENT = "recs-everyone"
SEED_SCRIPT = Path(__file__).resolve().parents[4] / "team-ai" / "scripts" / "seed_flipt_flags.py"
FLIPT_CONTAINER_ENV = "FLIPT_CONTAINER"
FLIPT_CONTAINER_DEFAULT = "agora-flipt-1"


def flipt_container() -> str:
    return os.getenv(FLIPT_CONTAINER_ENV, FLIPT_CONTAINER_DEFAULT)


def run_seed_script() -> subprocess.CompletedProcess[str]:
    """Run team-ai's idempotent flag seeding against the stack's Flipt."""
    return subprocess.run(
        [sys.executable, str(SEED_SCRIPT), "--url", get_settings().flipt_url.rstrip("/")],
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )


def get_flag(key: str = EXPERIMENT_FLAG) -> dict[str, Any] | None:
    with _client() as client:
        resp = client.get(f"/api/v1/namespaces/{NAMESPACE}/flags/{key}")
    if resp.status_code == httpx.codes.NOT_FOUND:
        return None
    resp.raise_for_status()
    return resp.json()


def delete_flag(key: str = EXPERIMENT_FLAG) -> None:
    with _client() as client:
        resp = client.delete(f"/api/v1/namespaces/{NAMESPACE}/flags/{key}")
    if resp.status_code not in (httpx.codes.OK, httpx.codes.NO_CONTENT, httpx.codes.NOT_FOUND):
        resp.raise_for_status()


def set_flag_enabled(enabled: bool, key: str = EXPERIMENT_FLAG) -> None:
    flag = get_flag(key)
    assert flag is not None, f"flag {key} does not exist"
    body = {k: flag[k] for k in ("key", "name", "description") if k in flag}
    body["enabled"] = enabled
    with _client() as client:
        client.put(f"/api/v1/namespaces/{NAMESPACE}/flags/{key}", json=body).raise_for_status()


def set_even_rollout(key: str = EXPERIMENT_FLAG) -> None:
    """One rule over a match-everything segment splitting the flag's variants evenly."""
    flag = get_flag(key)
    assert flag is not None, f"flag {key} does not exist"
    variants = flag.get("variants") or []
    assert len(variants) >= 2, f"flag {key} has fewer than two variants: {variants}"
    base = f"/api/v1/namespaces/{NAMESPACE}"
    with _client() as client:
        client.post(
            f"{base}/segments",
            json={"key": EXPERIMENT_SEGMENT, "name": EXPERIMENT_SEGMENT, "matchType": "ALL_MATCH_TYPE"},
        )
        for rule in client.get(f"{base}/flags/{key}/rules").json().get("rules", []):
            client.delete(f"{base}/flags/{key}/rules/{rule['id']}")
        rule = client.post(
            f"{base}/flags/{key}/rules", json={"segmentKey": EXPERIMENT_SEGMENT, "rank": 1}
        )
        rule.raise_for_status()
        rule_id = rule.json()["id"]
        share = 100.0 / len(variants)
        for variant in variants:
            client.post(
                f"{base}/flags/{key}/rules/{rule_id}/distributions",
                json={"variantId": variant["id"], "rollout": share},
            ).raise_for_status()


def restore_experiment_flag_disabled() -> None:
    """Teardown: the flag is back to its seeded state, disabled and without rules."""
    delete_flag()
    run_seed_script()


def wait_flipt_healthy(timeout_s: float = 60.0) -> None:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        try:
            with _client() as client:
                if client.get("/health").status_code == httpx.codes.OK:
                    return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise AssertionError("flipt did not become healthy again")
