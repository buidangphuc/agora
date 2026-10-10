## 1. Code — team-search

- [x] 1.1 Change the `HYBRID_SEMANTIC_MIN_SCORE` default to 0.65 in config.go, .env.example and the README, and update
      the README rationale with the measurement. Verify with a config default test and `make check`.
- [x] 1.2 Commit the probe as `scripts/semantic_floor_probe.py`.

## 2. E2E

- [x] 2.1 The overlay keeps 0.3. Re-run the hybrid floor scenarios after rebuilding team-search; they must stay
      green.

## Evidence (2026-10-10)

- Final gate on HEAD 6b714354: parallel lane (`e2e.sh -q -n 4 -m "not destructive"`) 825/825 passed, run twice; destructive lane 103/104. The one failure, `test_c1_notification_faults::test_name_lookup_failure_still_notifies`, belongs to change C1 (not this change) and passed on isolated rerun (flaky). Stack READY.
