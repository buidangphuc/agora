# Runbook: alerts and the local alert channel

Pipeline: Prometheus rules (`platform-gitops/platform/monitoring/alerts.yaml`) -> Alertmanager (`alertmanager.yaml`, one route) -> `webhook-logger` (`webhook-logger.yaml`, prints one JSON line per alert).

| Alert | Fires when | Metric | Severity |
|---|---|---|---|
| `HighErrorRate` | edge gRPC non-OK ratio > 5% for 2m and > 0.05 req/s | `marketplace_rpc_client_duration_milliseconds_count` | critical |
| `HighLatencyP95` | edge p95 > 500 ms for 5m | `marketplace_rpc_client_duration_milliseconds_bucket` | warning |
| `ServiceDown` | `up{job=~"team-ai\|otel-collector"} == 0` for 1m | scrape `up` | critical |

The gateway RED series only exist while the gateway exports OTLP to `otel-collector` (`OTEL_ENABLED=true`: set in the staging-on-kind overlay, not in the plain local gateway).

## Read fired alerts
`kubectl -n marketplace logs deploy/webhook-logger --tail=50` (lines with `"event": "alert"`, `status` firing/resolved, `runbook`).

## Prove the channel works (no workload is touched)
```bash
kubectl -n marketplace port-forward svc/alertmanager 9093:9093 &
curl -s -XPOST localhost:9093/api/v2/alerts -H 'Content-Type: application/json' -d \
 '[{"labels":{"alertname":"ServiceDown","job":"drill","severity":"critical"},"annotations":{"summary":"alert drill","runbook":"docs/runbooks/alerts.md"}}]'
sleep 15; kubectl -n marketplace logs deploy/webhook-logger --tail=5     # expect a firing ServiceDown line
kill %1
```
Real firing (human-approved): `kubectl -n marketplace scale deploy/team-ai --replicas=0` is reverted by ArgoCD/KEDA, so prefer the synthetic alert above, or stop the pod's `/metrics` by scaling otel-collector to 0 for 90s and let `ServiceDown{job="otel-collector"}` fire (ArgoCD selfHeal will restore it afterwards).

Check rules/state: `kubectl -n marketplace port-forward svc/prometheus 9090:9090` then `http://localhost:9090/alerts`.

## Drill log
<!-- Fill during the live run: date, method (synthetic / real), alert, time to appear in webhook-logger. -->

| Date | Operator | Method | Alert | Time to webhook-logger | Result |
|---|---|---|---|---|---|
| | | | | | |
