import copy
import json

import httpx

from scripts.seed_flipt_flags import FLAG_KEY, main, seed_flags

BASE = "http://flipt.test:8080"
FLAGS_PATH = "/api/v1/namespaces/default/flags"


class FakeFlipt:
    def __init__(self, flags=None):
        self.flags = flags or {}
        self.writes = []

    def handler(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        body = json.loads(request.content) if request.content else {}
        if request.method != "GET":
            self.writes.append((request.method, path, body))
        if request.method == "GET" and path == FLAGS_PATH:
            return httpx.Response(200, json={"flags": list(self.flags.values())})
        if path.startswith(FLAGS_PATH + "/"):
            rest = path[len(FLAGS_PATH) + 1 :].split("/")
            key = rest[0]
            if request.method == "GET" and len(rest) == 1:
                if key not in self.flags:
                    return httpx.Response(404, json={"message": "not found"})
                return httpx.Response(200, json=self.flags[key])
            if request.method == "POST" and rest[1:] == ["variants"]:
                if key not in self.flags:
                    return httpx.Response(404, json={"message": "not found"})
                self.flags[key]["variants"].append(body)
                return httpx.Response(200, json=body)
            if request.method in ("PUT", "DELETE"):
                return httpx.Response(200, json={})
        if request.method == "POST" and path == FLAGS_PATH:
            if body["key"] in self.flags:
                return httpx.Response(409, json={"message": "exists"})
            self.flags[body["key"]] = {
                **body,
                "enabled": bool(body.get("enabled", False)),
                "variants": [],
            }
            return httpx.Response(200, json=self.flags[body["key"]])
        return httpx.Response(404, json={"message": "unrouted"})

    def client(self) -> httpx.Client:
        return httpx.Client(base_url=BASE, transport=httpx.MockTransport(self.handler))


def test_seeding_creates_a_disabled_flag_with_both_variants():
    fake = FakeFlipt()

    seed_flags(fake.client())

    flag = fake.flags[FLAG_KEY]
    assert FLAG_KEY == "recs-home-feed-ranker"
    assert flag["enabled"] is False
    assert {v["key"] for v in flag["variants"]} == {"control", "similarity"}


def test_seeding_creates_a_variant_flag_type():
    fake = FakeFlipt()

    seed_flags(fake.client())

    assert fake.flags[FLAG_KEY]["type"] == "VARIANT_FLAG_TYPE"


def test_similarity_variant_carries_the_ranker_attachment_and_control_none():
    fake = FakeFlipt()

    seed_flags(fake.client())

    variants = {v["key"]: v for v in fake.flags[FLAG_KEY]["variants"]}
    assert json.loads(variants["similarity"]["attachment"]) == {
        "ranking": {"model": "cosine_rank"}
    }
    assert not variants["control"].get("attachment")


def test_seeding_never_sends_an_enable_request():
    fake = FakeFlipt()

    seed_flags(fake.client())

    for _method, _path, body in fake.writes:
        assert body.get("enabled", False) is False


def test_seeding_twice_is_idempotent_and_the_second_run_writes_nothing():
    fake = FakeFlipt()
    seed_flags(fake.client())
    after_first = copy.deepcopy(fake.flags)
    writes_first = len(fake.writes)

    seed_flags(fake.client())

    assert fake.flags == after_first
    assert len(fake.writes) == writes_first
    assert len(fake.flags[FLAG_KEY]["variants"]) == 2


def test_existing_flag_untouched():
    existing = {
        FLAG_KEY: {
            "key": FLAG_KEY,
            "name": FLAG_KEY,
            "type": "VARIANT_FLAG_TYPE",
            "enabled": True,
            "variants": [{"key": "control"}, {"key": "similarity", "attachment": "{}"}],
            "rollouts": [{"distribution": 50}],
        }
    }
    fake = FakeFlipt(copy.deepcopy(existing))

    seed_flags(fake.client())

    assert fake.flags == existing
    assert fake.flags[FLAG_KEY]["enabled"] is True
    assert fake.writes == []


def test_existing_partial_flag_is_not_completed_or_modified():
    existing = {
        FLAG_KEY: {
            "key": FLAG_KEY,
            "type": "VARIANT_FLAG_TYPE",
            "enabled": False,
            "variants": [],
        }
    }
    fake = FakeFlipt(copy.deepcopy(existing))

    seed_flags(fake.client())

    assert fake.flags == existing
    assert fake.writes == []


def test_main_exits_zero_on_a_repeat_run(monkeypatch):
    fake = FakeFlipt()
    monkeypatch.setattr(
        "scripts.seed_flipt_flags.build_client",
        lambda url: httpx.Client(
            base_url=url, transport=httpx.MockTransport(fake.handler)
        ),
    )

    assert main(["--url", BASE]) == 0
    assert main(["--url", BASE]) == 0
    assert fake.flags[FLAG_KEY]["enabled"] is False
    assert len(fake.flags[FLAG_KEY]["variants"]) == 2
