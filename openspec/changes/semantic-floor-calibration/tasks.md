## 1. Code — team-search

- [x] 1.1 Change the `HYBRID_SEMANTIC_MIN_SCORE` default to 0.65 in config.go, .env.example and the README, and update
      the README rationale with the measurement. Verify with a config default test and `make check`.
- [x] 1.2 Commit the probe as `scripts/semantic_floor_probe.py`.

## 2. E2E

- [ ] 2.1 The overlay keeps 0.3. Re-run the hybrid floor scenarios after rebuilding team-search; they must stay
      green.
