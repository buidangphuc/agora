# Outbox first rollout (chat, payment, engagement, domain, order, identity)

The first rollout of an outbox service must never run two relayers. The old pods are
stopped first, the PreSync `migrate up` Job runs, then the new pods start. Promotion has no
old relayer and needs no scale-down.

Applications come from the ApplicationSet, so no CLI `--set` exists: the only way to pass the
strategy is a one-line overlay edit in `envs/<env>/services/<svc>.yaml`.

## Steps

1. Stop auto-sync and `selfHeal` for the Application, so neither can revert the manual scale-down:
   `argocd app set <svc>-<env> --sync-policy none`.
2. Scale the old Deployment to zero and wait until no pod of it runs:
   `kubectl scale deployment/<svc> -n marketplace --replicas=0`, then
   `kubectl wait --for=delete pod -l app=<svc> -n marketplace --timeout=120s`.
3. Commit the image bump together with the overlay line `strategy: { type: Recreate }` in
   `envs/<env>/services/<svc>.yaml` (a guard against a stray old pod coming back, for example from
   an HPA) and run one manual `argocd app sync <svc>-<env>`. The PreSync Job `<svc>-migrate` runs
   first (requeues parked outbox rows), then the new pods start.
4. Re-enable auto-sync and `selfHeal` (`argocd app set <svc>-<env> --sync-policy automated
   --self-heal`), after a follow-up commit removes the `strategy` line.

## If the migrate Job fails in this first rollout

The service has an outage: the old pods are already at zero, so nothing serves traffic while
the Job is failing or the sync is stuck. Restore service within 30 minutes:

1. Read the Job logs (`kubectl logs job/<svc>-migrate -n marketplace`), fix the cause and re-sync
   (`argocd app sync <svc>-<env>`). A Job is bounded to 2 retries and 600 seconds, so it fails fast.
2. If the fix does not land in time, scale the previous Deployment back on the old image
   (`kubectl scale deployment/<svc> -n marketplace --replicas=<previous>` with the old tag) and
   keep auto-sync off. Only do this while the new outbox table is empty: run
   `SELECT count(*) FROM outbox;` (the outbox table of that service) and proceed only when it
   prints 0, because the old relayer ignores the `seq` column and would republish or skip rows.
3. After recovery, repeat the steps above from step 1.

## Later rollouts

A failed PreSync Job fails the sync and the existing pods keep running. No scale-down is needed.
