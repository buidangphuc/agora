"""Black-box helpers for the hand-off code scenarios (change identity-sso-handoff).

Everything goes through the gateway (Connect JSON), the two Next apps over HTTP, the audit
log through QueryAuditLog, and read-only SQL on identity_db for the persisted rows.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import re
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from email.utils import parsedate_to_datetime
from html.parser import HTMLParser

import httpx

from config.settings import get_settings
from tests.e2e.support import audit_support as aud
from tests.e2e.support import user_enforcement_support as ue

PKG = "/platform.identity.v1.AuthService"
MINT = f"{PKG}/CreateHandoffCode"
PEEK = f"{PKG}/PeekHandoffCode"
REDEEM = f"{PKG}/RedeemHandoffCode"
REFRESH = f"{PKG}/RefreshSession"
SELLER = "HANDOFF_APP_SELLER"
BUYER = "HANDOFF_APP_BUYER"
UNSPECIFIED = "HANDOFF_APP_UNSPECIFIED"
TTL_S = 60
CODE_RE = re.compile(r"^[A-Za-z0-9_-]{43}$")
MINT_ACTION = "auth.handoff_mint"
REDEEM_ACTION = "auth.handoff_redeem"


@dataclass
class Outcome:
    status: int
    code: str
    message: str
    body: dict
    elapsed_s: float
    date: float | None

    @property
    def ok(self) -> bool:
        return self.status == 200

    @property
    def failure(self) -> tuple[int, str, str]:
        return (self.status, self.code, self.message)


def rpc(path: str, body: dict, token: str | None = None) -> Outcome:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    started = time.perf_counter()
    resp = httpx.post(f"{get_settings().gateway_url.rstrip('/')}{path}", json=body, headers=headers, timeout=20)
    elapsed = time.perf_counter() - started
    try:
        data = resp.json()
    except ValueError:
        data = {}
    if not isinstance(data, dict):
        data = {}
    stamp = resp.headers.get("date")
    return Outcome(
        status=resp.status_code,
        code=str(data.get("code", "")),
        message=str(data.get("message", "")),
        body=data,
        elapsed_s=elapsed,
        date=parsedate_to_datetime(stamp).timestamp() if stamp else None,
    )


def mint(token: str | None, app: str = SELLER) -> Outcome:
    return rpc(MINT, {"targetApp": app}, token)


def peek(code: str, app: str = SELLER) -> Outcome:
    return rpc(PEEK, {"code": code, "targetApp": app})


def redeem(code: str, app: str = SELLER) -> Outcome:
    return rpc(REDEEM, {"code": code, "targetApp": app})


def minted_code(out: Outcome) -> str:
    assert out.ok, f"mint failed: {out.status} {out.body}"
    return out.body.get("code", "")


def result_of(out: Outcome) -> dict:
    assert out.ok, f"redeem failed: {out.status} {out.body}"
    return out.body.get("result") or {}


def new_user(role: str = "buyer", prefix: str = "ho") -> dict:
    return ue.register_user(role, prefix)


def mint_code(user: dict, app: str = SELLER) -> str:
    return minted_code(mint(user["token"], app))


def revoke_source(user: dict) -> None:
    from src.api.services import SessionService
    from tests.e2e.flows import session_id_of

    svc = SessionService(token=user["token"])
    try:
        svc.revoke_session(session_id_of(user["token"]))
    finally:
        svc.close()


def refresh(refresh_token: str) -> Outcome:
    return rpc(REFRESH, {"refreshToken": refresh_token})


def concurrently(n: int, fn) -> list:
    with ThreadPoolExecutor(max_workers=n) as pool:
        return list(pool.map(lambda _: fn(), range(n)))


def _psql(sql: str) -> str:
    container = os.getenv("POSTGRES_CONTAINER", "agora-postgres-1")
    proc = subprocess.run(
        ["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", "identity_db", "-Atq", "-v", "ON_ERROR_STOP=1"],
        input=sql,
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert proc.returncode == 0, f"identity_db query failed: {proc.stderr.strip()[:300]}"
    return proc.stdout.strip()


def rows_for(user_id: str) -> int:
    return int(_psql(f"SELECT count(*) FROM handoff_codes WHERE user_id::text = '{user_id}';"))


def live_rows_for(user_id: str) -> int:
    return int(
        _psql(
            "SELECT count(*) FROM handoff_codes WHERE "
            f"user_id::text = '{user_id}' AND redeemed_at IS NULL AND expires_at >= now();"
        )
    )


def rows_with_hash(code: str) -> int:
    digest = hashlib.sha256(code.encode()).hexdigest()
    return int(_psql(f"SELECT count(*) FROM handoff_codes WHERE code_hash = decode('{digest}', 'hex');"))


def rows_containing_plaintext(code: str) -> int:
    return int(_psql(f"SELECT count(*) FROM handoff_codes t WHERE t::text LIKE '%{code}%';"))


def redeemed_at_is_null(code: str) -> bool:
    digest = hashlib.sha256(code.encode()).hexdigest()
    return _psql(f"SELECT redeemed_at IS NULL FROM handoff_codes WHERE code_hash = decode('{digest}', 'hex');") == "t"


def audit_entries(world, user_id: str, action: str) -> list[dict]:
    resp = aud.query_events(aud.admin_token(world), actor_id=user_id, action=action)
    assert resp.status_code == 200, f"QueryAuditLog failed: {resp.status_code} {resp.text[:200]}"
    return [e for e in resp.json().get("events", []) if e.get("action") == action]


def audit_total(world, user_id: str) -> int:
    resp = aud.query_events(aud.admin_token(world), actor_id=user_id)
    assert resp.status_code == 200, f"QueryAuditLog failed: {resp.status_code} {resp.text[:200]}"
    return len(resp.json().get("events", []))


def forbidden_forms(code: str, user_id: str, sid: str) -> list[str]:
    digest = hashlib.sha256(code.encode())
    return [
        code,
        digest.hexdigest(),
        base64.b64encode(digest.digest()).decode(),
        base64.urlsafe_b64encode(digest.digest()).decode().rstrip("="),
        user_id,
        sid,
    ]


def container_logs(container: str, since_epoch: float) -> str:
    proc = subprocess.run(
        ["docker", "logs", "--since", str(int(since_epoch)), container],
        capture_output=True,
        text=True,
        timeout=60,
    )
    return proc.stdout + proc.stderr


def jwt_claims(token: str) -> dict:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))


class _FormParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.forms: list[dict] = []
        self.external: list[str] = []
        self._text: list[str] = []
        self._skip = False

    def handle_starttag(self, tag, attrs):  # noqa: ANN001
        a = {k: (v or "") for k, v in attrs}
        if tag == "form":
            self.forms.append({"method": a.get("method", "get").lower(), "action": a.get("action", ""), "fields": {}})
        elif tag == "input" and self.forms and a.get("name"):
            self.forms[-1]["fields"][a["name"]] = a.get("value", "")
        if tag in ("script", "link", "img", "iframe", "source", "video", "audio"):
            ref = a.get("src") or a.get("href") or ""
            if tag == "link" and a.get("rel", "") not in ("stylesheet", "preload", "icon"):
                ref = ""
            if re.match(r"^(https?:)?//", ref):
                self.external.append(ref)
        if tag in ("script", "style"):
            self._skip = True

    def handle_endtag(self, tag):  # noqa: ANN001
        if tag in ("script", "style"):
            self._skip = False

    def handle_data(self, data):  # noqa: ANN001
        if not self._skip:
            self._text.append(data)


def parse_page(html: str) -> _FormParser:
    parser = _FormParser()
    parser.feed(html)
    return parser


def page_text(html: str) -> str:
    return re.sub(r"\s+", " ", " ".join(parse_page(html)._text)).strip()


def cookie_header(cookies: dict[str, str]) -> str:
    return "; ".join(f"{k}={v}" for k, v in cookies.items())


def app_client() -> httpx.Client:
    return httpx.Client(follow_redirects=False, timeout=20)


def cookies_set(resp: httpx.Response) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in resp.headers.get_list("set-cookie"):
        name, _, rest = line.partition("=")
        out[name.strip()] = rest.split(";", 1)[0]
    return out


def session_cookies_set(resp: httpx.Response) -> dict[str, str]:
    return {k: v for k, v in cookies_set(resp).items() if v and k.endswith(("_session", "_refresh", "session"))}


def app_origin(app: str) -> str:
    s = get_settings()
    return (s.seller_base_url if app == "seller" else s.admin_base_url if app == "admin" else s.base_url).rstrip("/")


def handoff_url(app: str, code: str, next_path: str | None = None) -> str:
    q = f"?code={code}"
    if next_path is not None:
        q += f"&next={next_path}"
    return f"{app_origin(app)}/auth/handoff{q}"


def post_handoff(
    app: str,
    fields: dict[str, str],
    headers: dict[str, str] | None = None,
    cookies: dict[str, str] | None = None,
) -> httpx.Response:
    sent = dict(headers or {})
    if cookies:
        sent["Cookie"] = cookie_header(cookies)
    host = sent.pop("Host", None)
    with app_client() as client:
        req = client.build_request("POST", f"{app_origin(app)}/auth/handoff", data=fields, headers=sent)
        if host:
            req.headers["Host"] = host
        return client.send(req)


def confirmation_fields(app: str, code: str, next_path: str, cookies: dict[str, str] | None = None) -> dict[str, str]:
    """The hidden body fields of the confirmation form, exactly as the page renders them."""
    with app_client() as client:
        resp = client.get(handoff_url(app, code, next_path), headers={"Cookie": cookie_header(cookies)} if cookies else {})
    assert resp.status_code == 200, f"confirmation page expected, got {resp.status_code} {resp.headers.get('location')}"
    forms = [f for f in parse_page(resp.text).forms if f["method"] == "post"]
    assert forms, "confirmation page has no POST form"
    return forms[0]["fields"]


def own_origin_headers(app: str) -> dict[str, str]:
    return {"Origin": app_origin(app)}


def location_of(resp: httpx.Response) -> str:
    return resp.headers.get("location", "")


def wait_until_age(minted_at: float, age_s: float) -> None:
    remaining = minted_at + age_s - time.time()
    if remaining > 0:
        time.sleep(remaining)
