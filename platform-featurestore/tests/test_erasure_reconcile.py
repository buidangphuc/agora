"""AUE-24 / AUE-27: online reconcile of user_activity and user_preferences before the pointers."""

import json

import fakeredis
import redis as redis_lib
from conftest import d, h, write_inputs
from test_materialize import ev

from featurestore import cli

ERASED = "erased:9f2c01ab"


def listing(i, category):
    return {"id": i, "category": category, "price": 10, "at": d(3), "seller": "s1"}


def _inputs(dirs, users):
    events = [ev(f"e-{u}", "view", u, "L1", h(2)) for u in users]
    write_inputs(dirs[0], events, listings=[listing("L1", "cat-a")])


def _keys(r, pattern):
    return set(r.scan_iter(pattern))


def test_erased_user_disappears_from_online_store_AUE_24(dirs, env, redis, capsys):
    _inputs(dirs, ["U", "V"])
    assert cli.main(["materialize"], env, redis) == 0
    assert redis.get("fs:user_activity:v2:U") and redis.get("fs:user_preferences:v1:U")

    _inputs(dirs, [ERASED, "V"])
    assert cli.main(["materialize"], env, redis) == 0
    assert redis.get("fs:user_activity:v2:U") is None
    assert redis.get("fs:user_preferences:v1:U") is None
    assert not _keys(redis, "fs:*:erased:*")
    assert redis.get("fs:user_activity:v2:V") and redis.get("fs:user_preferences:v1:V")
    capsys.readouterr()
    assert cli.main(["parity"], env, redis) == 0


def test_stale_user_keys_from_previous_run_are_removed_AUE_24(dirs, env, redis):
    _inputs(dirs, ["V"])
    redis.set("fs:user_activity:v2:U", json.dumps({"views_7d": 3}))
    redis.set("fs:user_preferences:v1:U", json.dumps({"preferred_categories": "cat-a"}))
    assert cli.main(["materialize"], env, redis) == 0
    assert redis.get("fs:user_activity:v2:U") is None
    assert redis.get("fs:user_preferences:v1:U") is None
    assert redis.get("fs:user_activity:v2:V")


def test_reconcile_leaves_other_prefixes_alone_AUE_24(dirs, env, redis):
    _inputs(dirs, ["V"])
    redis.set("fs:user_activity_other:v2:U", "keep")
    redis.set("chat:session:U:1", "keep")
    assert cli.main(["materialize"], env, redis) == 0
    assert redis.get("fs:user_activity_other:v2:U") == "keep"
    assert redis.get("chat:session:U:1") == "keep"


class _FailingDelete(fakeredis.FakeRedis):
    def _boom(self, *a, **k):
        raise redis_lib.exceptions.ConnectionError("redis down during reconcile")

    delete = _boom
    unlink = _boom

    def pipeline(self, *a, **k):
        p = super().pipeline(*a, **k)
        p.delete = self._boom
        p.unlink = self._boom
        return p


def test_redis_failure_during_reconcile_fails_run_AUE_27(dirs, env):
    _inputs(dirs, ["V"])
    r = _FailingDelete(decode_responses=True)
    r.set("fs:user_activity:v2:U", json.dumps({"views_7d": 3}))
    r.set("fs:user_preferences:v1:U", json.dumps({"preferred_categories": "cat-a"}))
    try:
        code = cli.main(["materialize"], env, r)
    except redis_lib.exceptions.RedisError:
        code = 1
    assert code != 0
    for view in ("user_activity", "user_preferences"):
        assert r.get(f"fs:{view}:current") is None
        assert r.get(f"fs:{view}:meta") is None
