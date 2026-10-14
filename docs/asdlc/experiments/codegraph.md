# CodeGraph experiment (W5-1 sibling: W5-5)

Question: does CodeGraph save tokens on this polyrepo, and does it cover Go? Measured 2026-10-11 with
`codegraph` CLI (same output as the `codegraph_explore` MCP tool) on the existing index.

## Go support: confirmed

`codegraph status` (index of `/agora`): 2,473 files, 31,546 nodes, 82,078 edges. Files by language:
python 972, **go 610**, tsx 427, yaml 248, typescript 195. Go symbols are present as `struct` (738),
`method` (3,786 shared with other languages) and `function`. Go answers return real symbols: the team-order
question below returned `team-order/internal/repository/order.go`, `.../saga.go` with structs, methods and call edges.
Caveats: index is stale (38 added + 5 modified files pending, built by engine v1.5.0; run `codegraph sync`);
`proto`-generated `*.pb.go` are indexed too and add noise.

## Method

Three questions, one per language/team. For each:

- **A. grep+read baseline** (what an agent does without the index): `grep -rEc <keywords>` over the team dir
  (go/py/ts/tsx only; excluding tests, `node_modules`, `.venv`, `.uv-cache`, `generated/`), take the 5 files with
  most hits, read them whole. Cost = bytes of those 5 files. This is a floor for A: a real agent also pays for
  the grep output, wrong files and follow-up reads.
- **B. CodeGraph**: one `codegraph explore "<question>"` call. Cost = bytes of its output (it is capped near 25 KB).
- Tokens ~= bytes / 4 (rough, same factor both sides).

Reproduce:

```bash
codegraph explore "how does team-order cart add item and checkout saga work" | wc -c
grep -rIcE "AddToCart|Checkout|[Ss]aga" team-order --include=*.go --exclude-dir=generated --exclude=*_test.go | sort -t: -k2 -nr | head -5
```

## Results

| # | Team (lang) | Question | A: top-5 files read | B: codegraph | Relevance of B (manual, first look) |
|---|---|---|---|---|---|
| 1 | team-order (Go) | cart add item and checkout saga | 104 KB ~ 26k tok (17 matching files) | 25 KB ~ 6.3k tok | Good: `repository/saga.go`, `order.go`; but also pulled `team-frontend` checkout files |
| 2 | team-ai (Python) | LLM inference retry and fallback | 71 KB ~ 17.7k tok (32 matching files) | 25 KB ~ 6.3k tok | Good: `llm/router.py` (fallback_models, breaker); noise: `platform-e2e` page object, `langfuse.py` |
| 3 | team-frontend (TS) | checkout page calls order API | 23 KB ~ 5.7k tok (29 matching files) | 26 KB ~ 6.5k tok | Mixed: `checkoutParams.ts` ok; includes `platform-e2e` service classes (wrong repo) |

Without the exclusions the naive baseline is much worse (245 KB for #1 from `*.pb.go`; 347 KB for #2 from `.uv-cache`),
so the saving depends heavily on whether the agent already knows to skip generated code.

## Reading

- CodeGraph is **flat at ~6k tokens per question**. It wins when grep matches many large files (Go order service,
  Python AI router: 2.8x to 4x fewer tokens than reading 5 files) and loses on a small, well-named TS feature
  (#3: grep found 5 small files, 0.9x).
- It does not scope by team: the query should name the repo dir, otherwise `platform-e2e` and other teams leak in.
- Value beyond tokens: call paths and blast radius in one call (useful for reviewers checking write-sets).
- Not measured (TODO, needs live sessions): end-to-end task token cost and answer correctness with vs without
  CodeGraph; number of tool round-trips; `codegraph sync` cost after large diffs.

## Verdict

Keep CodeGraph for Go and Python locate/understand questions and for blast-radius checks in reviewer agents;
do not require it for small TS features. Next step if wanted: run 3 real tasks (one per language) with
`codegraph_explore` allowed vs denied and compare the session token totals from `metrics/runs/*.jsonl`.
