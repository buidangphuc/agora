# Feature Manifest — proposal & schema

> Status: **proposal (schema only)**. No loader yet, no repo files created yet.
> This defines the convention so we can review it before rolling out.

## Why (the problem you raised)

Every repo already has a `README.md`, but those are **repo-understanding** docs —
organized by technical layer (RPC catalog, DB schema, route list). They answer
"what is this service?" They do **not** give an automation agent a clean,
feature-by-feature contract it can turn into an E2E test. `MASTER_CAPABILITY_MATRIX.md`
is closer (10 business phases) but it's prose Markdown for humans, not machine input.

For **auto E2E generation** to be reliable, the generator needs, per feature: who
the actor is, where the journey starts, which RPCs/preconditions set it up, the key
selectors, and acceptance criteria already phrased as Given/When/Then. That is what
this manifest captures.

The manifest **does not replace the README** — it adds a thin, structured,
feature-centric layer next to it.

## The convention

- **One file per repo**: `<repo>/FEATURES.yaml`, owned by that repo's team, so it
  stays in sync with the code (unlike a central file that drifts).
- **platform-e2e aggregates** all `*/FEATURES.yaml`, validates each against
  [`schemas/features.schema.json`](../schemas/features.schema.json), and feeds the
  auto-generator. (Aggregator/loader is a later step — not built in this pass.)
- A feature is owned by the repo that **owns the capability**, even when the UI
  lives in `team-frontend` (e.g. `order.purchase-saga` → `team-order`). `services:`
  lists every repo the journey touches. Purely presentational UI → `team-frontend`.

## Field reference (summary — schema is authoritative)

| Field | Purpose for automation |
|---|---|
| `id` | Stable `<domain>.<slug>` key; coverage traceability across the polyrepo. |
| `persona` | Which account the generator logs in as (`preconditions.login` may override). |
| `priority` | `P0` → eligible for `@smoke`. |
| `entry_route` | Where the generated scenario navigates first (`{param}` allowed). |
| `services` | The set of repos that must be up for this test to run. |
| `preconditions.login` / `.seed_tag` | Login persona + a `TAG_PAYLOAD_MAP` marker that seeds state via the gateway API. |
| `api.seed` / `.exercised` | RPCs (`<service>.v1.<Method>`) for setup and for coverage/API assertions. |
| `ui.key_elements` | `name → semantic-locator` (our `role:`/`label:`/`text:`… DSL) → seeds page objects. |
| `acceptance` | Gherkin-ready lines → become the Scenario body. |
| `tags` | Become pytest markers. |
| `status` | `planned` (ready for auto-gen) · `automated` · `manual` · `not-testable`. |
| `covered_by` | Set to `features/<file>.feature::<Scenario>` once automated → closes the loop. |

## How `auto` will consume it (target pipeline — not built yet)

```
<repo>/FEATURES.yaml ──▶ platform-e2e loader
                          ├─ validate vs features.schema.json
                          ├─ for each feature status: planned
                          │    ├─ emit tests/e2e/features/<persona>/<id>.feature   (from acceptance + tags)
                          │    ├─ scaffold missing steps + page object (entry_route + ui.key_elements)
                          │    └─ wire seeding from preconditions.seed_tag
                          ├─ run the generated scenario
                          └─ on green: flip status→automated, set covered_by
```

Coverage report = diff between manifested features and `covered_by` pointers →
"what's specified but not yet automated". The aggregator can also **regenerate**
`MASTER_CAPABILITY_MATRIX.md` from the manifests, so humans and machines share one
source instead of two hand-maintained docs.

## Worked example

[`examples/team-order.FEATURES.yaml`](examples/team-order.FEATURES.yaml) — four
real team-order features (cart, purchase saga, saga compensation, RMA) grounded in
the actual RPCs, routes and selectors. Validate it with any JSON-Schema tool, e.g.:

```bash
pip install check-jsonschema
check-jsonschema --schemafile schemas/features.schema.json docs/examples/team-order.FEATURES.yaml
```

## Authoring gotcha

Inside a YAML flow list, quote any path containing `{param}` — `{` starts a flow
mapping otherwise: `related_routes: ["/checkout/pay/{order_id}", /cart]`.
Plain scalars (`entry_route: /listing/{listing_id}`) are fine unquoted.

## Proposed rollout (after this schema is approved)

1. Approve schema + field set (this doc).
2. Add `FEATURES.yaml` to 2 pilot repos (`team-frontend`, `team-order`), back-filling
   `status: automated` + `covered_by` for the scenarios platform-e2e already runs.
3. Build the loader/validator + a `make features-check` gate in platform-e2e.
4. Roll out to the remaining repos; wire generation + the coverage report.
