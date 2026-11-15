"""Helpers for change identity-data-export (initiative trust-legal).

Black box through the gateway (Connect JSON). Seeding uses real RPCs only (Login, CreateAddress,
RecordConsent, RequestAccountDeletion, CreateHandoffCode); one identity_db SELECT reads the
password hash so the export bytes can be checked for it.
"""

from __future__ import annotations

import base64
import json
from concurrent.futures import ThreadPoolExecutor

import httpx

from src.constants import legal_versions as legal
from tests.e2e.support import account_deletion_support as ad
from tests.e2e.support import handoff_support as ho
from tests.e2e.support import user_enforcement_support as ue

EXPORT = "/platform.identity.v1.PrivacyService/ExportMyData"
CREATE_ADDRESS = "/platform.identity.v1.AddressService/CreateAddress"
RECORD_CONSENT = "/platform.identity.v1.PrivacyService/RecordConsent"
LIST_CONSENTS = "/platform.identity.v1.PrivacyService/ListMyConsents"

TOP_KEYS = {
    "schema_version", "generated_at", "account", "addresses", "sessions",
    "login_history", "consents", "deletion", "truncated",
}  # fmt: skip
MAX_BYTES = 1048576
LOGIN_CAP = 1000


def export(token: str | None) -> httpx.Response:
    return ue.call(EXPORT, {}, token)


def document(resp: httpx.Response) -> dict:
    assert resp.status_code == 200, f"ExportMyData {resp.status_code}: {resp.text[:300]}"
    return json.loads(raw_bytes(resp))


def raw_bytes(resp: httpx.Response) -> bytes:
    data = ue.body_of(resp).get("data", "")
    return base64.b64decode(data + "=" * (-len(data) % 4))


def add_addresses(user: dict, n: int) -> list[str]:
    ids = []
    for i in range(n):
        body = {
            "recipientName": f"Export Probe {i}", "phone": "0912345678",
            "street": f"{i + 1} Lieu Giai", "ward": "Phuong Lieu Giai",
            "district": "Quan Ba Dinh", "city": "Ha Noi", "isDefault": i == 0,
        }  # fmt: skip
        resp = ue.call(CREATE_ADDRESS, body, user["token"])
        assert resp.status_code == 200, f"CreateAddress {resp.status_code}: {resp.text[:300]}"
        ids.append(ue.body_of(resp)["address"]["id"])
    return ids


def record_consents(user: dict, purposes: list[str]) -> None:
    records = [
        {"purpose": p, "granted": True, "version": legal.CURRENT_TERMS, "source": "e2e_export"}
        for p in purposes
    ]
    resp = ue.call(RECORD_CONSENT, {"records": records}, user["token"])
    assert resp.status_code == 200, f"RecordConsent {resp.status_code}: {resp.text[:300]}"


def consent_total(user: dict) -> int:
    resp = ue.call(LIST_CONSENTS, {"page": {"pageSize": 100}}, user["token"])
    assert resp.status_code == 200, f"ListMyConsents {resp.status_code}: {resp.text[:300]}"
    return len(ue.body_of(resp).get("records", []))


def extra_logins(user: dict, ok: int, failed: int = 0) -> None:
    for _ in range(ok):
        ue.login(user)
    for _ in range(failed):
        assert ue.login_raw(user["username"], "wrong-" + user["password"]).status_code != 200


def many_logins(user: dict, n: int, workers: int = 8) -> None:
    with ThreadPoolExecutor(max_workers=workers) as pool:
        results = list(pool.map(lambda _: ue.login_raw(user["username"], user["password"]), range(n)))
    bad = [r for r in results if r.status_code != 200]
    assert not bad, f"{len(bad)} of {n} logins failed: {bad[0].status_code} {bad[0].text[:200]}"


def password_hash(user_id: str) -> str:
    return ad.user_row(user_id)["password_hash"]


def mint_handoff(user: dict) -> str:
    return ho.mint_code(user)
