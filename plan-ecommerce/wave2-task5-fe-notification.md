# [W2-T5] frontend — price/stock alert subscriptions UI

## Role
SE

## Objective
On a listing, the user can toggle "Notify me on price drop / back in stock"; a preferences area lists active alert subscriptions; the notification center shows price-drop / back-in-stock notifications.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/notifications/**         (edit — show alert notifications + manage subscriptions)
- team-frontend/src/features/notification/**     (edit)
- team-frontend/src/lib/gateway/notification.ts  (create/edit — alert RPCs)
- team-frontend/src/components/alerts/**          (create — AlertToggle rendered by listing slot)

## Read-only dependencies
- team-frontend/src/generated/** (notification stubs, Wave 0)
- listing/[id]/page.tsx slot (Wave 0 renders AlertToggle; DO NOT edit the shell)
- existing notifications page + bell component

## Contracts
Server-only gateway module. Subscribe/Unsubscribe/List alerts; notification center already lists notifications — add the new types' rendering (icon/link).

## Acceptance criteria
- [ ] Alert toggle on a listing subscribes/unsubscribes; state reflects existing subscription.
- [ ] Subscriptions manageable from the notifications/preferences area.
- [ ] Price-drop / back-in-stock notifications render with correct link; typechecks/build pass.

## Review (gate — different agent)
Peer agent against SE rubric.

## Verify
```bash
# (cd team-frontend && npm run typecheck && npm run build)
```

## Out of scope
- Do NOT edit shared shells (Wave 0) or other features' dirs. Gateway-only data access.
