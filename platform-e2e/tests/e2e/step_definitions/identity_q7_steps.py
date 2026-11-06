from __future__ import annotations

import json
import os
import time
import uuid
from datetime import datetime, timezone

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.constants import gateway_endpoints as ep
from src.utils import data as fake
from tests.e2e.flows import wait_for_gateway
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import user_enforcement_support as ue
from tests.e2e.support.world import World

GET_MY_ACCOUNT = f"{ue.IDENTITY}.AuthService/GetMyAccount"
USER_REGISTERED = "platform.identity.v1.UserRegistered"
EVENT_WAIT_S = 20.0
QUIET_S = 6.0
LOG_SETTLE_S = 6.0
SPOOF_HEADERS = ("x-user-id", "x-principal-id", "x-principal-user-id", "x-forwarded-user")


def _x(world: World) -> dict:
    return world.state.extra


def _container(env: str, default: str) -> str:
    return os.getenv(env, default)


class RegisteredTap:
    TOPIC = "identity.events"

    def __init__(self) -> None:
        from confluent_kafka import Consumer, TopicPartition

        self._consumer = Consumer(
            {
                "bootstrap.servers": os.getenv("KAFKA_BROKERS", "localhost:19092"),
                "group.id": f"e2e-q7-{time.time_ns()}",
                "enable.auto.commit": False,
            }
        )
        meta = self._consumer.list_topics(self.TOPIC, timeout=15)
        assigns = []
        for p in meta.topics[self.TOPIC].partitions:
            _, high = self._consumer.get_watermark_offsets(
                TopicPartition(self.TOPIC, p), timeout=15
            )
            assigns.append(TopicPartition(self.TOPIC, p, high))
        self._consumer.assign(assigns)
        self.seen: list[dict] = []

    def _drain(self, wait_s: float) -> None:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            msg = self._consumer.poll(0.5)
            if msg is None or msg.error():
                continue
            env = ue._fields(msg.value())
            if env.get(2, [b""])[0].decode() != USER_REGISTERED:
                continue
            payload = ue._fields(env[7][0])
            registered = ue._fields(payload[3][0]).get(1, [0])[0] if 3 in payload else 0
            self.seen.append(
                {
                    "key": (msg.key() or b"").decode(errors="ignore"),
                    "event_id": env.get(1, [b""])[0].decode(),
                    "user_id": payload.get(1, [b""])[0].decode(),
                    "referral_code": payload.get(2, [b""])[0].decode(),
                    "registered_at": registered,
                }
            )

    def for_user(self, user_id: str, wait_s: float, at_least: int = 1) -> list[dict]:
        deadline = time.monotonic() + wait_s
        while time.monotonic() < deadline:
            found = [e for e in self.seen if e["user_id"] == user_id]
            if len(found) >= at_least:
                return found
            self._drain(1.0)
        return [e for e in self.seen if e["user_id"] == user_id]

    def settle(self, wait_s: float) -> None:
        self._drain(wait_s)

    def close(self) -> None:
        self._consumer.close()


def _tap(world: World) -> RegisteredTap:
    x = _x(world)
    if "tap" not in x:
        x["tap"] = RegisteredTap()
        world.add_cleanup(x["tap"].close)
    return x["tap"]


def _register(
    world: World,
    username: str | None = None,
    code: str | None = None,
    extra: dict | None = None,
    role: str = "buyer",
) -> dict:
    _tap(world)
    username = username or fake.unique_username("q7reg")
    body: dict = {"username": username, "password": get_settings().seed_password, "role": role}
    if code is not None:
        body["referralCode"] = code
    if extra:
        body.update(extra)
    started = time.perf_counter()
    resp = ue.call(ep.AUTH_REGISTER, body)
    elapsed = time.perf_counter() - started
    token = ue.body_of(resp).get("token", "") if resp.status_code == 200 else ""
    result = {
        "username": username,
        "code": code,
        "resp": resp,
        "elapsed_s": elapsed,
        "token": token,
        "user_id": ue.user_id_of(token) if token else "",
        "registered_wall": time.time(),
    }
    _x(world).setdefault("regs", []).append(result)
    return result


def _last(world: World) -> dict:
    return _x(world)["regs"][-1]


def _shape(value):
    if isinstance(value, dict):
        return {k: _shape(v) for k, v in sorted(value.items())}
    if isinstance(value, list):
        return [_shape(v) for v in value[:1]]
    return type(value).__name__


def _event_for(world: World, reg: dict) -> dict:
    events = _tap(world).for_user(reg["user_id"], EVENT_WAIT_S)
    assert events, f"no UserRegistered event for user {reg['user_id']} within {EVENT_WAIT_S}s"
    return events[0]


def _parse_ts(value: str) -> float:
    base, _, rest = value.rstrip("Z").partition(".")
    frac = (rest[:6]).ljust(6, "0") if rest else "000000"
    dt = datetime.strptime(f"{base}.{frac}", "%Y-%m-%dT%H:%M:%S.%f").replace(tzinfo=timezone.utc)
    return dt.timestamp()


def _account_of(token: str) -> httpx.Response:
    return ue.call(GET_MY_ACCOUNT, {}, token)


def _utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


@given(parsers.parse('a visitor registered a username with referral code "{code}"'))
def given_registered_with_code(world: World, code: str) -> None:
    _x(world)["log_since"] = _utc_now()
    reg = _register(world, code=code)
    assert reg["resp"].status_code == 200, reg["resp"].text
    _x(world)["original"] = reg


@when(parsers.parse('a visitor registers a new username with referral code "{code}"'))
def when_register_with_code(world: World, code: str) -> None:
    _register(world, code=code)


@when("a visitor registers a new username with no referral code")
def when_register_no_code(world: World) -> None:
    _register(world)


@when("a visitor registers a new username with a unique referral code")
def when_register_unique_code(world: World) -> None:
    _x(world)["log_since"] = _utc_now()
    code = f"LG{uuid.uuid4().hex[:14]}"
    _x(world)["unique_code"] = code
    _register(world, code=code)


@when('a visitor registers with role "admin" and a client-supplied user id')
def when_register_admin_role(world: World) -> None:
    spoofed = f"client-{uuid.uuid4().hex[:10]}"
    _x(world)["spoofed_id"] = spoofed
    _register(world, code="ROLE1", role="admin", extra={"userId": spoofed, "id": spoofed})


@when(
    "one visitor registers with a valid code of 32 characters and another with a "
    "valid-charset code of 33 characters"
)
def when_register_boundary(world: World) -> None:
    code32 = ("B32" + uuid.uuid4().hex + uuid.uuid4().hex)[:32]
    code33 = code32 + "x"
    assert len(code32) == 32 and len(code33) == 33
    _x(world)["code32"] = code32
    _register(world, code=code32)
    _register(world, code=code33)


@when("a visitor registers with a well-formed code that belongs to no user")
def when_register_unknown_code(world: World) -> None:
    _register(world, code=f"NOSUCH{uuid.uuid4().hex[:8]}")


@when("another visitor registers with no code")
def when_register_other_no_code(world: World) -> None:
    _register(world)


@when('another visitor registers the same username with referral code "DUP2"')
def when_register_duplicate(world: World) -> None:
    _register(world, username=_x(world)["original"]["username"], code="DUP2")


@when("another visitor registers the same username with a unique referral code")
def when_register_duplicate_unique(world: World) -> None:
    _x(world)["log_since"] = _utc_now()
    code = f"LF{uuid.uuid4().hex[:14]}"
    _x(world)["unique_code"] = code
    _register(world, username=_x(world)["original"]["username"], code=code)


@when("an anonymous client calls Register at the gateway with a referral code")
def when_anonymous_register(world: World) -> None:
    _register(world, code="ANON1")


@then("Register succeeds")
def then_register_succeeds(world: World) -> None:
    resp = _last(world)["resp"]
    assert resp.status_code == 200, f"Register answered {resp.status_code}: {resp.text}"
    assert _last(world)["user_id"], "Register returned no token to read the user id from"


@then("Register succeeds with the same response shape as without a code")
def then_register_same_shape(world: World) -> None:
    then_register_succeeds(world)
    reg = _last(world)
    baseline = _register(world)
    assert baseline["resp"].status_code == 200, baseline["resp"].text
    assert _shape(reg["resp"].json()) == _shape(baseline["resp"].json()), (
        f"response shape differs: {reg['resp'].text[:300]} vs {baseline['resp'].text[:300]}"
    )
    _x(world)["regs"].remove(baseline)


@then(parsers.parse('the UserRegistered event for the new user carries referral code "{code}"'))
def then_event_code(world: World, code: str) -> None:
    reg = _last(world)
    assert reg["user_id"], f"Register failed: {reg['resp'].status_code} {reg['resp'].text}"
    event = _event_for(world, reg)
    assert event["referral_code"] == code, (
        f"payload code {event['referral_code']!r}, wanted {code!r}"
    )
    assert event["key"] == reg["user_id"]


@then("the UserRegistered event for the new user carries an empty referral code")
def then_event_empty(world: World) -> None:
    reg = _last(world)
    assert reg["user_id"], f"Register failed: {reg['resp'].status_code} {reg['resp'].text}"
    event = _event_for(world, reg)
    assert event["referral_code"] == "", f"payload code {event['referral_code']!r}, wanted empty"


@then("the created account is a buyer with a server-generated user id")
def then_buyer_account(world: World) -> None:
    reg = _last(world)
    claims = ue.claims(reg["token"])
    assert reg["user_id"] != _x(world)["spoofed_id"], "the client-supplied user id was used"
    assert "admin" not in json.dumps(claims).lower(), f"token carries admin: {claims}"
    assert "buyer" in json.dumps(claims).lower(), f"token does not carry buyer: {claims}"


@then("the UserRegistered event user id equals the generated id")
def then_event_user_id(world: World) -> None:
    reg = _last(world)
    assert _event_for(world, reg)["user_id"] == reg["user_id"]


@then("both Register calls succeed")
def then_both_succeed(world: World) -> None:
    for reg in _x(world)["regs"][-2:]:
        assert reg["resp"].status_code == 200, reg["resp"].text


@then("the first UserRegistered event carries the 32-character code")
def then_first_32(world: World) -> None:
    event = _event_for(world, _x(world)["regs"][-2])
    assert event["referral_code"] == _x(world)["code32"]


@then("the second UserRegistered event carries an empty referral code")
def then_second_empty(world: World) -> None:
    assert _event_for(world, _x(world)["regs"][-1])["referral_code"] == ""


@then("both Register responses have the same status code and body shape")
def then_parity(world: World) -> None:
    a, b = _x(world)["regs"][-2:]
    assert a["resp"].status_code == b["resp"].status_code == 200
    assert _shape(a["resp"].json()) == _shape(b["resp"].json())


@then("exactly one UserRegistered event for the new user is published")
def then_exactly_one(world: World) -> None:
    reg = _last(world)
    _event_for(world, reg)
    tap = _tap(world)
    tap.settle(QUIET_S)
    events = [e for e in tap.seen if e["user_id"] == reg["user_id"]]
    assert len(events) == 1, f"{len(events)} UserRegistered events for {reg['user_id']}"


@then("its registered_at is within 5 seconds of the account's created_at")
def then_registered_at_close(world: World) -> None:
    reg = _last(world)
    event = _event_for(world, reg)
    resp = _account_of(reg["token"])
    assert resp.status_code == 200, resp.text
    created = _parse_ts(ue.body_of(resp)["createdAt"])
    assert abs(event["registered_at"] - created) <= 5, (event["registered_at"], created)


@then("Register returns the conflict error")
def then_conflict(world: World) -> None:
    resp = _last(world)["resp"]
    assert resp.status_code != 200, "duplicate Register succeeded"
    assert ue.code_of(resp).lower() == "already_exists", resp.text


@then("still exactly one UserRegistered event exists for the original user")
def then_original_single(world: World) -> None:
    original = _x(world)["original"]
    tap = _tap(world)
    assert tap.for_user(original["user_id"], EVENT_WAIT_S)
    tap.settle(QUIET_S)
    events = [e for e in tap.seen if e["user_id"] == original["user_id"]]
    assert len(events) == 1, f"{len(events)} events for the original user"


@then(parsers.parse('no UserRegistered event carries referral code "{code}"'))
def then_no_event_with_code(world: World, code: str) -> None:
    assert not [e for e in _tap(world).seen if e["referral_code"] == code]


@then("the gateway answers with the Register result and not 401 or 403")
def then_public(world: World) -> None:
    resp = _last(world)["resp"]
    assert resp.status_code not in (401, 403), resp.text
    assert resp.status_code == 200, resp.text


def _stop_targets() -> list[str]:
    return [
        _container("REFERRAL_CONTAINER", "agora-team-referral-svc"),
        _container("PLP_TEAM_PROMOTION_CONTAINER", "agora-team-promotion-svc"),
        _container("KAFKA_CONTAINER", "agora-redpanda-1"),
    ]


@given("team-referral, team-promotion and Kafka are stopped")
def given_downstream_stopped(world: World) -> None:
    _tap(world)
    stopped: list[str] = []

    def restore() -> None:
        for name in reversed(stopped):
            pe.docker("start", name, check=False)
        wait_for_gateway(get_settings().gateway_url)

    world.add_cleanup(restore)
    for name in _stop_targets():
        pe.docker("stop", name)
        stopped.append(name)


@then("Register succeeds within the existing Register latency budget")
def then_within_budget(world: World) -> None:
    then_register_succeeds(world)
    budget = float(os.getenv("REGISTER_LATENCY_BUDGET_S", "3.0"))
    assert _last(world)["elapsed_s"] <= budget, f"Register took {_last(world)['elapsed_s']:.2f}s"


@when("Kafka is started again")
def when_kafka_started(world: World) -> None:
    pe.docker("start", _container("KAFKA_CONTAINER", "agora-redpanda-1"))


@when("the identity and gateway logs are searched")
def when_logs_searched(world: World) -> None:
    time.sleep(LOG_SETTLE_S)
    since = _x(world)["log_since"]
    text = ""
    for name in (
        _container("IDENTITY_CONTAINER", "agora-team-identity-svc"),
        pe.gateway_container(),
    ):
        proc = pe.docker("logs", "--since", since, name, check=False)
        text += proc.stdout + proc.stderr
    _x(world)["log_text"] = text


@then("neither the new user id nor the referral code appears in any log line")
def then_logs_clean_new(world: World) -> None:
    text = _x(world)["log_text"]
    assert text.strip(), "no log lines captured; the search would pass vacuously"
    assert _last(world)["user_id"] not in text
    assert _x(world)["unique_code"] not in text


@then("neither the original user id nor the referral code appears in any log line")
def then_logs_clean_original(world: World) -> None:
    text = _x(world)["log_text"]
    assert text.strip(), "no log lines captured; the search would pass vacuously"
    assert _x(world)["original"]["user_id"] not in text
    assert _x(world)["unique_code"] not in text


@when("50 visitors register with a well-formed referral code")
def when_fifty(world: World) -> None:
    latencies = []
    for i in range(50):
        reg = _register(world, code=f"PERF{i:03d}")
        assert reg["resp"].status_code == 200, reg["resp"].text
        latencies.append(reg["elapsed_s"] * 1000)
    latencies.sort()
    _x(world)["p95_ms"] = latencies[int(0.95 * len(latencies)) - 1]


@then("the p95 Register latency is at most 10 ms above the recorded baseline")
def then_p95(world: World) -> None:
    raw = os.getenv("REGISTER_BASELINE_P95_MS")
    assert raw, "REGISTER_BASELINE_P95_MS (recorded Register p95 before this change) is not set"
    assert _x(world)["p95_ms"] <= float(raw) + 10, (
        f"p95 {_x(world)['p95_ms']:.1f} ms, baseline {raw} ms"
    )


def _register_role(world: World, role: str, key: str) -> None:
    user = ue.register_user(role, f"q7_{role}")
    user["registered_wall"] = time.time()
    _x(world)[key] = user


@given("a registered buyer")
def given_buyer(world: World) -> None:
    _register_role(world, "buyer", "buyer")


@given("a second registered buyer")
def given_second_buyer(world: World) -> None:
    _register_role(world, "buyer", "buyer2")


@given("a registered seller")
def given_seller(world: World) -> None:
    _register_role(world, "seller", "seller")


@given("the buyer's user row is removed")
def given_row_removed(world: World) -> None:
    ue._psql(f"DELETE FROM users WHERE id = '{_x(world)['buyer']['id']}';")


@when("the buyer calls GetMyAccount through the gateway")
def when_buyer_account(world: World) -> None:
    _x(world)["account"] = _account_of(_x(world)["buyer"]["token"])


@when("a caller with no token calls GetMyAccount at the gateway")
def when_anonymous_account(world: World) -> None:
    _x(world)["account"] = ue.call(GET_MY_ACCOUNT, {}, None)


@when("the second buyer calls GetMyAccount naming the first buyer's id in a header and in the body")
def when_spoof_account(world: World) -> None:
    first = _x(world)["buyer"]["id"]
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"bearer {_x(world)['buyer2']['token']}",
    }
    headers.update({name: first for name in SPOOF_HEADERS})
    _x(world)["account"] = httpx.post(
        f"{get_settings().gateway_url.rstrip('/')}{GET_MY_ACCOUNT}",
        json={"userId": first, "user_id": first},
        headers=headers,
        timeout=20,
    )


@when("the seller and the buyer each call GetMyAccount through the gateway")
def when_both_account(world: World) -> None:
    _x(world)["seller_account"] = _account_of(_x(world)["seller"]["token"])
    _x(world)["buyer_account"] = _account_of(_x(world)["buyer"]["token"])


@then("the response user id is that buyer's id")
def then_own_id(world: World) -> None:
    resp = _x(world)["account"]
    assert resp.status_code == 200, resp.text
    assert ue.body_of(resp)["userId"] == _x(world)["buyer"]["id"]


@then("created_at is within 5 seconds of the registration time")
def then_created_at(world: World) -> None:
    created = _parse_ts(ue.body_of(_x(world)["account"])["createdAt"])
    assert abs(created - _x(world)["buyer"]["registered_wall"]) <= 5, created


@then("the gateway answers 401")
def then_401(world: World) -> None:
    resp = _x(world)["account"]
    assert resp.status_code == 401, f"{resp.status_code} {resp.text}"


@then("the response user id is the second buyer's own id")
def then_second_own_id(world: World) -> None:
    resp = _x(world)["account"]
    assert resp.status_code == 200, resp.text
    assert ue.body_of(resp)["userId"] == _x(world)["buyer2"]["id"]


@then("nothing of the first buyer's account is returned")
def then_nothing_of_first(world: World) -> None:
    assert _x(world)["buyer"]["id"] not in _x(world)["account"].text


@then("each response carries that caller's own user id")
def then_each_own(world: World) -> None:
    for key, resp_key in (("seller", "seller_account"), ("buyer", "buyer_account")):
        resp = _x(world)[resp_key]
        assert resp.status_code == 200, f"{key}: {resp.status_code} {resp.text}"
        assert ue.body_of(resp)["userId"] == _x(world)[key]["id"]


@then("the call fails with not_found and returns no account data")
def then_not_found(world: World) -> None:
    resp = _x(world)["account"]
    assert resp.status_code == 404, f"{resp.status_code} {resp.text}"
    assert ue.code_of(resp).lower() == "not_found"
    assert _x(world)["buyer"]["id"] not in resp.text
