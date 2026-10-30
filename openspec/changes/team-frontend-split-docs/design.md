## Decisions

- Manifest ownership follows the route: an entry moves to `team-frontend-seller` when its notes name a test in that app or its `entry_route` is under `/seller` or `/sell`, and to `team-frontend-admin` when its notes name a test in that app or its `entry_route` is under `/admin`. Evidence wins over the route, so the ten `frontend.listing-moderation-*` entries routed at `/admin/moderation` but verified by seller tests go to the seller manifest. The three `frontend.dce-*` browser scenarios (route `/`, admin persona) go with the other dispute-cases entries to the admin manifest. Login, chat, notifications and review-hide entries stay in the buyer manifest because those routes are served by the buyer app.
- The move is done by a script over the text blocks so entry formatting is kept byte for byte; only `services: [team-frontend, ...]` is rewritten to the owning app. Section comments stay in the buyer manifest when it keeps entries of that section; the new manifests carry no comments.
- Stale spec paths are rewritten only where the target file exists in exactly one app, checked by a script before the rewrite.

## Risks

- A future change that adds a seller or admin entry to `team-frontend/FEATURES.yaml` is not caught by any gate; `docs/agents/project.md` records the ownership rule.
- `cross_change_check.py` matches scenario titles across manifests; moving an entry keeps exactly one owner, so no title is duplicated.
