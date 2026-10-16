# feature-run frontend-ai-interaction-events
initiative: q4-loop-instrumentation | base: d34f0cc8 | worktree: .claude/worktrees/frontend-ai-interaction-events | branch: asdlc/frontend-ai-interaction-events
## Preflight   validate=p spec_lint=p
## E2E         author=done run=red as expected (16/16: no ai_interaction event; 3 negatives with positive control) files=4 (+FEATURES 22)
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| 1.1-1.6 (batched) | 6 unit + 16 e2e | approved | 2 | d4b3389c | PASS (type-only test fix Red-First-Exempt) | pii | approve | – |
Round 2: thread switch disarms copilot edited (tasks.md 1.3), red test 25c66ac5 → green 1d762916. e2e send-button locator fixed (matched a chip containing "gửi").
## Gates       e2e=16/16 green (rebuilt image) features-check=p spec_sync --strict=p (16 e2e + 6 unit) feature-reviewer=pass 22/22 privacy=needs-human decide=n/a (LLM fallback)
## Escalations Spec defect (spec-clarify Round 2, human): nobody added ai_interaction to schema.ts/map.ts → this slice adds it; tasks.md 1.1 write-set widened; re-locked a76237442d7b. Audit lists tasks.md/LOCK.md/packet (orchestrator re-lock edits).
Drift: spec VERIFIED BY + tasks.md 2.2 list 11 "AI events: …" names; feature uses the 22 spec titles → human at converge.
Privacy needs-human: DPO lawful basis for first-party beacons ("legitimate interest" is not a Decree 13 basis); analytics.events retention covers ai_interaction.
