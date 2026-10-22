# team-ai-ingress-domain-caller

initiative: adhoc

## Why
team-domain calls `AIService.ScreenListing` on team-ai to screen a listing before it is published (`UPSTREAM_AI_ADDR`). PR #41 (`d746e088`) therefore admitted `app: team-domain` in the team-ai ingress NetworkPolicy, but `deploy-runtime` and `recs-on-cluster` still say the allow-list is exactly `team-gateway`, `team-search-indexer` and `prometheus`. The two static e2e checks of that rule fail on main. Without team-domain in the list, listing publish on the cluster fails (screening unreachable).

## What changes
- `deploy-runtime`: the team-ai ingress allow-list adds `app: team-domain`, the caller of `AIService.ScreenListing`.
- `recs-on-cluster`: the team-ai allow-list it pins is the same four pods.
- The e2e scenarios `team-ai renders an ingress allow-list` (pear_authz_gitops) and `team-ai keeps its ingress allow-list` (roc_recs_on_cluster), their bindings and the FEATURES.yaml acceptance lines name team-domain.
- No manifest change: platform-gitops already admits team-domain.

## NFR
- No runtime change; the policy already ships with team-domain.

## Data classification
No personal data is added, stored, logged or sent. The change only documents an existing network path.

## Threat model
- Asset: team-ai (LLM proxy, recommendation reads) reachable only from its callers.
- Abuse case: a compromised team-domain pod can call team-ai. ScreenListing and the other team-ai RPCs still enforce scopes on the forwarded principal; team-domain calls with its service principal.
- Every other pod stays refused; non-pod ingress sources stay forbidden.
