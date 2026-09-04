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

import httpx

from config.settings import get_settings

FLAG_KEY = "checkout-enabled"
NAMESPACE = "default"


def _client() -> httpx.Client:
    return httpx.Client(base_url=get_settings().flipt_url.rstrip("/"), timeout=15.0)


def set_checkout_flag(enabled: bool) -> bool:
    """Set `checkout-enabled` to `enabled` in Flipt. Returns the applied state.

    Ensures the boolean flag exists (creating it on first run), then updates its
    `enabled` state. Idempotent: re-running with the same value is a no-op flip.
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
        # Update in place; create it if this env has never seen the flag.
        resp = client.put(f"{base}/{FLAG_KEY}", json=payload)
        if resp.status_code == httpx.codes.NOT_FOUND:
            client.post(base, json=payload)
    return enabled
