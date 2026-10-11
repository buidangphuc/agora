import sys
from datetime import datetime, timezone
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import baseline as b  # noqa: E402


def T(day, hour=0):
    return datetime(2026, 10, day, hour, tzinfo=timezone.utc)


def C(sha, ts, subject="x", files=(), parents=1, body=""):
    return b.Commit(sha, ts, parents, subject, body, list(files))


def test_percentile():
    assert b.percentile([], 50) is None
    assert b.percentile([5], 90) == 5.0
    assert b.percentile([1, 2, 3, 4], 50) == 2.5
    assert b.percentile([4, 1, 3, 2], 75) == 3.25
    assert b.percentile([1, 2, 3, 4, 5], 90) == pytest.approx(4.6)


@pytest.mark.parametrize("subject,expected", [
    ("fix(team-ai): x", True), ("fix: x", True), ("Fix(team-ai): x", True),
    ("fix!: breaking", True), ("fix typo", True),
    ("feat: fixes things", False), ("fixture(x): y", False), ("chore: fix", False),
    ("prefix: fix", False), ("", False),
])
def test_is_fix_commit(subject, expected):
    assert b.is_fix_commit(subject) is expected


def test_parse_ts_to_utc():
    assert b.parse_ts("2026-10-11T13:15:04+07:00") == datetime(2026, 10, 11, 6, 15, 4, tzinfo=timezone.utc)


def test_parse_log():
    raw = ("\x1eaaa\x1f2026-10-01T10:00:00+07:00\x1f\x1fspec(x): propose\x1f\x1f\nA\topenspec/changes/x/proposal.md\n"
           "M\tteam-ai/a.py\n"
           "\x1ebbb\x1f2026-10-02T10:00:00+07:00\x1faaa ccc\x1fmerge: y\x1fbody text\x1f\n")
    cs = b.parse_log(raw)
    assert [c.sha for c in cs] == ["aaa", "bbb"]
    assert cs[0].parents == 0 and cs[1].parents == 2
    assert cs[0].files == [("A", "openspec/changes/x/proposal.md"), ("M", "team-ai/a.py")]
    assert cs[1].body == "body text" and cs[1].files == []


def test_archive_dirname_and_top_dir():
    assert b.parse_archive_dirname("2026-10-09-foo-bar") == ("2026-10-09", "foo-bar")
    assert b.parse_archive_dirname("weird") == (None, "weird")
    assert b.top_dir("team-ai/x/y.py") == "team-ai" and b.top_dir("README.md") is None


def test_repos_in_text():
    repos = {"team-ai", "deploy", "platform-core"}
    txt = "touches team-ai/src and platform-core, we deploy it; also team-ai-extra"
    assert b.repos_in_text(txt, repos) == {"team-ai", "platform-core"}
    assert b.repos_in_text("see deploy/helm", repos) == {"deploy"}


def test_first_add_picks_earliest_and_ignores_modify():
    cs = [C("b", T(3), files=[("A", "p/proposal.md")]),
          C("a", T(2), files=[("A", "p/proposal.md")]),
          C("c", T(1), files=[("M", "p/proposal.md")])]
    assert b.first_add(cs, lambda p: p == "p/proposal.md").sha == "a"
    assert b.first_add(cs, lambda p: p == "none") is None


def test_build_row_measured_with_fixes_and_window():
    repos = {"team-ai"}
    start = C("s", T(1), files=[("A", "openspec/changes/x/proposal.md")])
    end = C("e", T(3), files=[("A", "openspec/changes/archive/2026-10-03-x/proposal.md")])
    commits = [
        start, end,
        C("w1", T(2), "feat(team-ai): a", [("M", "team-ai/a.py")]),
        C("w2", T(2, 5), "feat(team-gateway): b", [("M", "team-gateway/a.py")]),   # other repo
        C("w3", T(2, 6), "merge: m", [], parents=2),                                # merge ignored
        C("f1", T(4), "fix(team-ai): bug", [("M", "team-ai/a.py")]),                # counted
        C("f2", T(5), "fix(team-gateway): bug", [("M", "team-gateway/a.py")]),      # other repo
        C("f3", T(6), "chore(team-ai): n", [("M", "team-ai/a.py")]),                # not fix
        C("f4", T(30), "fix(team-ai): late", [("M", "team-ai/a.py")]),              # outside 14d
        C("f0", T(2), "fix(team-ai): before", [("M", "team-ai/a.py")]),             # before end
    ]
    r = b.build_row("x", "2026-10-03-x", start, end, commits, repos, head_ts=T(31))
    assert r["cycle_status"] == "measured" and r["cycle_hours"] == 48.0 and r["cycle_days"] == 2.0
    assert r["post_merge_fixes"] == 1 and r["post_merge_fix_commits"] == ["f1"]
    assert r["commits_in_window"] == 2  # w1 and f0
    assert r["fix_window_complete"] is True
    r2 = b.build_row("x", "2026-10-03-x", start, end, commits, repos, head_ts=T(10))
    assert r2["fix_window_complete"] is False


def test_build_row_archive_only_and_same_instant():
    end = C("e", T(3))
    r = b.build_row("x", "d", None, end, [end], set(), T(30))
    assert r["cycle_status"] == "archive_only" and r["cycle_hours"] is None and r["commits_in_window"] is None
    r = b.build_row("x", "d", C("s", T(3)), end, [end], {"a"}, T(30))
    assert r["cycle_status"] == "same_instant" and r["cycle_hours"] is None


def test_summarize():
    def row(h, fixes, complete=True):
        return {"cycle_hours": h, "cycle_status": "measured" if h is not None else "archive_only",
                "post_merge_fixes": fixes, "fix_window_complete": complete}
    rows = [row(10, 0), row(20, 2), row(30, 0), row(None, 1), row(40, 5, complete=False)]
    s = b.summarize(rows)
    assert s["n_changes"] == 5 and s["n_cycle_measurable"] == 4 and s["n_archive_only"] == 1
    assert s["cycle_hours"]["median"] == 25.0
    assert s["n_fix_window_complete"] == 4
    assert s["fix_rate_changes_with_fix"] == 0.5
    assert b.summarize([])["cycle_hours"]["median"] is None


def test_mentions_id_word_boundary():
    assert b.mentions_id(C("a", T(1), "chore: archive foo-bar"), "foo-bar")
    assert not b.mentions_id(C("a", T(1), "chore: archive foo-bar-baz"), "foo-bar")
    assert b.mentions_id(C("a", T(1), "x", body="Refs: foo"), "foo")
