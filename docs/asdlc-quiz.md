# ASDLC v2 comprehension quiz (W0-1 verify)

A fresh subagent reads ONLY `docs/ASDLC-v2.md` and answers all 5; pass = 5/5 against the key. Answers must come from the doc, not the repo.

1. A veto question on the task gate fails in advisory mode, and the same unit is also flagged by script routing as touching PII. What is the decision result and what extra flag applies?
2. Which subagent is blocked from editing test files, which agents may edit them, and how does the hook know which subagent is calling? Name one kind of write the hook does not catch.
3. Kev is unreachable during a spec gate. What result does `decide.py` give, what is the verdict, and does the gate block?
4. Where exactly does the human lock the spec, what file records it, and what does `initiative-build` do if that file is not approved or a slice digest changed?
5. Which gate/tool checks red-first (tests fail before the change, pass after), what are its two modes, and which command runs it?

## Answer key

1. Advisory mode records the veto fail as `WOULD_BLOCK` (not a block); script routing adds the separate `NEEDS_HUMAN` flag, which a pass never clears.
2. `implementer` is blocked; `test-author` and `e2e-author` (and the main session) are not. The hook reads `agent_type` (the subagent name) from the PreToolUse stdin. Not caught: indirect writes such as `python -c` (Bash check is best-effort).
3. `ENGINE_DOWN`; the LLM reviewer's own checklist becomes the verdict, stated in the table; it does not block because infrastructure never blocks.
4. Step 4 of the initiative workflow, after spec review and `cross_change_check`: `initiative-plan` writes `plans/<id>/LOCK.md` and stops; the human signs it (`status: approved`, `approved-by:`, slices ticked). `initiative-build` refuses to start (STOP; the human re-locks via `initiative-plan` if a digest changed).
5. `scripts/asdlc/red_first.sh` (CI job `tdd`, run per task in `feature-run`). Unit/integration: new tests must fail at the base (merge-base) and pass at HEAD. E2E: the test commit must precede the implementation commit. Command: `scripts/asdlc/red_first.sh --base <ref>`.
