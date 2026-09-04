# [W7-T2] Observability wiring (platform-core infra)

## Role
SRE

## Objective
Bật obs profile; đẩy OTel traces→Jaeger, metrics→Prometheus; Grafana datasource + dashboard RPS/latency.
Nền cho cockpit (wave11) và trace spine (wave12).

## Write-set (EXCLUSIVE)
- platform-core/infra (edit — otel-collector exporter config Jaeger/Prometheus, compose profile obs)
- platform-core/docs/ADR/0004-observability-otel.md (edit — cập nhật trạng thái exporter đã wire)

## Read-only dependencies
- ADR-0004; 00-spec.md §Architecture (observability)

## Acceptance criteria
- [ ] `docker compose --profile obs up` bật jaeger:16686 + prometheus:9090 + grafana:3001
- [ ] OTel collector export traces→Jaeger, metrics→Prometheus (đổi CONFIG, không đổi code service)
- [ ] Grafana có datasource Prometheus + 1 dashboard RPS/latency p95/p99 khởi tạo

## Verify
docker compose --profile obs up; gọi 1 request → thấy trace trên Jaeger UI

## Out of scope
- KHÔNG instrument từng service ở đây (spans fold vào từng task Phase B); không đụng team-gateway repo
