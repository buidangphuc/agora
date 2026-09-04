# [W1] F8 AI review summarization (team-ai, Python)

## Role
SE

## Objective
Implement the RPC(s) below in **team-ai**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-ai/)
- team-ai/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: the existing magic-listing / completion module in app/ (LLM call + prompt + schema)

## Contracts you implement
`SummarizeReviews(listing_id, reviews[])` -> {summary, pros[], cons[], sentiment}. Reviews passed in the request (gateway gathers them); team-ai stays stateless.

## Reference implementation
Mirror **the existing magic-listing / completion module in app/ (LLM call + prompt + schema)** in team-ai: same handler/service/repository layering, same test layout, new domain. No DB. Add an app module + endpoint/servicer; deterministic test via a mocked LLM.

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-ai && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
