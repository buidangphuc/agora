"""Leave-last-new-item-out split: the evaluation can never see its own target."""

from recsys.evals.holdout import EVAL_PROTOCOL, leave_last_new_item_out
from recsys.recommend import top_n_unseen


def _ev(user, item, ts):
    return {"user_id": user, "listing_id": item, "timestamp": ts}


def test_target_is_the_latest_discovered_item_not_a_repeat():
    # u1 discovers l1, l2, then l3; the last event is a repeat of l1, which must not
    # become the target (it is already in the user's history).
    h = leave_last_new_item_out(
        [_ev("u1", "l1", 1), _ev("u1", "l2", 2), _ev("u1", "l3", 3), _ev("u1", "l1", 4)]
    )
    assert h.actual == {"u1": ["l3"]}
    assert h.cutoffs == {"u1": 3}


def test_training_keeps_only_events_before_the_target_was_discovered():
    h = leave_last_new_item_out(
        [_ev("u1", "l1", 1), _ev("u1", "l2", 2), _ev("u1", "l3", 3), _ev("u1", "l1", 4)]
    )
    # Events at ts 3 (the target) and 4 (after it) are out of training.
    assert h.train_events == 2
    assert h.test_events == 1
    assert h.cutoff_timestamp == 2


def test_users_without_a_new_item_only_train():
    h = leave_last_new_item_out(
        [
            _ev("solo", "l1", 1),
            _ev("solo", "l1", 5),  # one distinct item: nothing to hold out
            _ev("anonymous", "l1", 1),
            _ev("anonymous", "l2", 2),  # unkeyed rows are never held out
            _ev("same", "l1", 3),
            _ev("same", "l2", 3),  # all discovered at once: no "before" to train on
        ]
    )
    assert h.actual == {}
    assert h.test_events == 0


def test_seen_items_are_never_ranked():
    ranked = top_n_unseen(
        ["u1", "u2"],
        [[1.0, 0.0], [0.0, 1.0]],
        ["a", "b", "c"],
        [[1.0, 0.0], [0.9, 0.1], [0.0, 1.0]],
        top_n=3,
        seen={"u1": {"a"}},
        only_users={"u1"},
    )
    assert list(ranked) == ["u1"]
    assert [i for i, _ in ranked["u1"]] == ["b", "c"]


def test_protocol_is_versioned():
    assert EVAL_PROTOCOL == "leave-last-new-item-v1"
