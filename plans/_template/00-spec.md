# 00-spec — <initiative title>
<!-- plans/<id>/00-spec.md — initiative spec (ASDLC v2 plan section 13, step 1; built with
     parallel-scrum Steps 1-2.5). Copied into every team-spec-author's context: keep it short
     (<300 lines) and self-contained. Tag every unknown inline as an `UNRESOLVED: <question>` tag in square brackets;
     validate_plan.py refuses the plan while any tag remains or the Locked section is missing.
     This template deliberately has neither, so a copied-but-unconfirmed spec fails validation. -->

initiative: <id>

## Goal
<2-3 sentences: what is observably true for users when the initiative is done.>

## User stories
<Only the ones that change decisions. Given/When/Then happy path + top 1-2 edge cases each.
team-spec-author turns these into EARS scenarios per slice.>

## Non-goals
- <explicit exclusions that stop scope creep across slices>

## Architecture decisions
- <decision> — Why: <one line>. (New dependencies are decided only here.)

## Contracts
<Every RPC / message / event that crosses a slice boundary, written out. Each new item becomes
part of the platform-core contract slice (`provides:`); consumers cite it in `uses:`.>
```proto
// platform/<svc>/v1/<svc>.proto — add to <Service>
rpc <Rpc>(<Req>) returns (<Resp>);
```
Existing contracts consumed unchanged: <Service.Rpc, topic/Event, …>

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| <contract> | <id>-contract | platform-core | – | provides: … | platform-core |
| <capability> | <slice> | team-<x> | <id>-contract | uses: … | team-<x>, team-gateway, team-frontend |

Hot files edited by more than one slice (initiative-build serialises those slices):
- <path> — <slices>

## Out of initiative
<Work with no requirement change: one-line bug fixes, finishing a task of an existing change,
infra values. Done as plain tasks or inside the existing change, not as slices.>

## NFR and data (initiative level; each slice's proposal.md refines them)
- NFR: <latency / availability / limits with numbers>
- Personal data: <field — purpose, consent basis, retention> | none
- Risk routing expected: <auth | payment | pii | proto> (asdlc.config.yaml `routing`)

## Conventions
- <test location, mirror-this-pattern pointers, what not to touch>

<!-- Only after the human confirms ("chốt"), append a level-2 heading `Locked` followed by:
     `<YYYY-MM-DD> — Confirmed by <name>. <one-line summary of decisions + assumptions locked in>` -->
