"""Black-box access to the service-only flash-stock hold RPCs (change promotion-flash-stock-hold).

`ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` are not routed by the
gateway and team-promotion publishes no host port, so they are driven with `grpcurl` from a
throwaway container on the stack network against `team-promotion-svc:50061`, with the
`x-principal-*` headers the interceptor reads. Same pattern as `prm_pay_stack.grpc_refund`.

Names come from the environment with the compose defaults (see `plp_stack`):

    STACK_NETWORK                   docker network of the stack (default agora_default)
    FLASH_HOLD_PROMOTION_ADDR       gRPC address on that network (default team-promotion-svc:50061)
    FLASH_HOLD_GRPCURL_IMAGE        default fullstorydev/grpcurl:latest
    FLASH_HOLD_PROTO_DIR            proto root holding platform/promotion/v1/promotion.proto
                                    (default team-promotion/proto of this checkout)
    PLP_POSTGRES_CONTAINER          postgres container (promotion_db is read/backdated through psql)
"""

from __future__ import annotations

import json
import os
import re
import uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.support import plp_stack as stack

PROTO_FILE = "platform/promotion/v1/promotion.proto"
SERVICE = "platform.promotion.v1.FlashSaleService"
GATEWAY_FLASH = "/platform.promotion.v1.FlashSaleService/"
ORDER_PRINCIPAL = "service-team-order"
RESERVE_SCOPE = "promotion.reserve"

# (id, type, scopes) per principal kind the scenarios use.
PRINCIPALS: dict[str, tuple[str, str, str]] = {
    "service": (ORDER_PRINCIPAL, "service", RESERVE_SCOPE),
    "user": ("flash-hold-buyer", "user", ""),
    "user-with-scope": ("flash-hold-user-scoped", "user", f"{RESERVE_SCOPE},admin"),
    "service-no-scope": ("service-other", "service", ""),
}


def promotion_addr() -> str:
    return os.getenv("FLASH_HOLD_PROMOTION_ADDR", "team-promotion-svc:50061")


def grpcurl_image() -> str:
    return os.getenv("FLASH_HOLD_GRPCURL_IMAGE", "fullstorydev/grpcurl:latest")


def proto_dir() -> Path:
    env = os.getenv("FLASH_HOLD_PROTO_DIR")
    if env:
        return Path(env)
    return Path(__file__).resolve().parents[4] / "team-promotion" / "proto"


def _snake(grpc_code: str) -> str:
    """grpcurl's `PermissionDenied` -> the Connect code `permission_denied`."""
    return re.sub(r"(?<!^)(?=[A-Z])", "_", grpc_code).lower()


def call(method: str, body: dict[str, Any], principal: str | None = "service") -> tuple[str, Any]:
    """One FlashSaleService RPC: (connect-style code, response dict or error text).

    `principal` is a key of PRINCIPALS, or None for a call with no principal headers.
    The code is `ok` on success.
    """
    pdir = proto_dir()
    assert (pdir / PROTO_FILE).is_file(), f"promotion proto not found under {pdir} (FLASH_HOLD_PROTO_DIR)"
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        grpcurl_image(), "-plaintext", "-import-path", "/proto", "-proto", PROTO_FILE,
    ]  # fmt: skip
    if principal is not None:
        pid, ptype, scopes = PRINCIPALS[principal]
        cmd += ["-H", f"x-principal-id: {pid}", "-H", f"x-principal-type: {ptype}"]
        if scopes:
            cmd += ["-H", f"x-principal-scopes: {scopes}"]
    cmd += ["-d", json.dumps(body), promotion_addr(), f"{SERVICE}/{method}"]
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", json.loads(proc.stdout or "{}")
    err = (proc.stderr + proc.stdout).strip()
    m = re.search(r"Code:\s*(\w+)", err)
    return (_snake(m.group(1)) if m else f"grpcurl_exit_{proc.returncode}"), err


def reserve(
    reservation_id: str,
    campaign_id: str,
    listing_id: str,
    quantity: int,
    expected_sale_price: int,
    variant_id: str = "",
    principal: str | None = "service",
) -> tuple[str, Any]:
    return call(
        "ReserveFlashSaleStock",
        {
            "reservationId": reservation_id,
            "campaignId": campaign_id,
            "listingId": listing_id,
            "variantId": variant_id,
            "quantity": quantity,
            "expectedSalePrice": expected_sale_price,
        },
        principal,
    )


def commit(reservation_id: str, principal: str | None = "service") -> tuple[str, Any]:
    return call("CommitFlashSaleStock", {"reservationId": reservation_id}, principal)


def release(reservation_id: str, principal: str | None = "service") -> tuple[str, Any]:
    return call("ReleaseFlashSaleStock", {"reservationId": reservation_id}, principal)


def reserve_many(count: int, **kwargs: Any) -> list[tuple[str, Any]]:
    """`count` parallel single-unit reserves, each with its own reservation id.

    `kwargs` are `campaign_id`, `listing_id`, `expected_sale_price` and optionally `quantity`.
    """
    kwargs.setdefault("quantity", 1)
    ids = [f"flash-hold-{uuid.uuid4().hex[:12]}" for _ in range(count)]
    with ThreadPoolExecutor(max_workers=count) as pool:
        return list(pool.map(lambda rid: reserve(rid, **kwargs), ids))


# ── seeding and the stock meter, through the gateway ─────────────────────
def _post(token: str | None, path: str, body: dict[str, Any]) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{path}"
    return httpx.post(url, json=body, headers=headers, timeout=30)


def seed_flash_campaign(
    token: str,
    listing_id: str,
    stock_cap: int,
    sale_price: int,
    starts_at: str = "2026-09-01T00:00:00Z",
    ends_at: str = "2026-12-31T23:59:59Z",
) -> str:
    """CreateCampaign through the gateway; the new campaign id.

    `token` is the owning seller's (or an admin's). An admin call is audited first and fails closed
    with 503 when team-audit is not in the stack (promotion-admin-audit), so scenarios seed as the
    listing owner, which is audit-free.
    """
    resp = _post(
        token,
        GATEWAY_FLASH + "CreateCampaign",
        {
            "listingId": listing_id,
            "salePrice": sale_price,
            "stockCap": stock_cap,
            "startsAt": starts_at,
            "endsAt": ends_at,
        },
    )
    assert resp.status_code == 200, f"CreateCampaign failed {resp.status_code}: {resp.text}"
    campaign_id = (resp.json().get("campaign") or {}).get("id", "")
    assert campaign_id, f"CreateCampaign returned no campaign id: {resp.text}"
    return str(campaign_id)


def stock(campaign_id: str) -> dict[str, int]:
    """GetFlashSaleStock through the gateway: {"remaining": n, "stock_cap": n}."""
    resp = _post(None, GATEWAY_FLASH + "GetFlashSaleStock", {"campaignId": campaign_id})
    assert resp.status_code == 200, f"GetFlashSaleStock failed {resp.status_code}: {resp.text}"
    data = resp.json()
    # proto3 JSON omits zero values.
    return {
        "remaining": int(data.get("remaining", 0) or 0),
        "stock_cap": int(data.get("stockCap", data.get("stock_cap", 0)) or 0),
    }


def gateway_hold_call(token: str | None, method: str, body: dict[str, Any]) -> httpx.Response:
    """A hold RPC posted to the gateway Connect path (a browser's route)."""
    return _post(token, GATEWAY_FLASH + method, body)


# ── promotion_db through psql ────────────────────────────────────────────
def psql(sql: str, check: bool = True):
    """One statement in promotion_db as the superuser (rows as `a|b|c`, no headers)."""
    return stack.docker(
        "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", "promotion_db",
        "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        check=check,
    )  # fmt: skip


def sql_lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def hold_rows(reservation_id: str) -> list[list[str]]:
    out = psql(
        f"SELECT reservation_id, quantity, status FROM flash_sale_holds "
        f"WHERE reservation_id = {sql_lit(reservation_id)}"
    ).stdout.strip()
    return [line.split("|") for line in out.splitlines() if line]


def backdate_hold(reservation_id: str) -> None:
    """Make the hold 15+ minutes old: its expires_at moves into the past."""
    psql(
        f"UPDATE flash_sale_holds SET expires_at = now() - interval '1 minute' "
        f"WHERE reservation_id = {sql_lit(reservation_id)}"
    )
