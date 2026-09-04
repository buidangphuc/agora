# BẢN THIẾT KẾ KIẾN TRÚC HẠ TẦNG TOÀN DIỆN (CLOUD & DEVOPS ARCHITECTURE)

Tài liệu thiết kế kiến trúc hạ tầng cho sàn thương mại điện tử quy mô lớn (Shopee-like Polyrepo Platform), phục vụ cả môi trường **Local Development** và **Production Cloud (Kubernetes)**.

---

## 🏛️ 1. TỔNG QUAN MÔ HÌNH HẠ TẦNG (HIGH-LEVEL INFRASTRUCTURE)

```
                            INTERNET (Buyers, Sellers, Mobile Apps)
                                       │
                                       ▼ (HTTPS / HTTP/2 / gRPC-Web)
                     ┌───────────────────────────────────┐
                     │    Cloudflare / AWS CloudFront    │  (DDoS, CDN, WAF, SSL)
                     └─────────────────┬─────────────────┘
                                       │
                                       ▼
                     ┌───────────────────────────────────┐
                     │    Envoy Gateway / Ingress NGINX  │  (Rate Limit, CORS, Auth Edge)
                     └─────────────────┬─────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────┴──────────────────────────────────────────┐
│                             KUBERNETES CLUSTER (EKS / GKE)                      │
│                                                                                 │
│   ┌────────────────┐      ┌────────────────┐      ┌─────────────────────────┐   │
│   │ team-frontend  │      │  team-gateway  │      │  team-ai (FastAPI)      │   │
│   │ (Next.js SSR)  │─────►│ (Connect Edge) │─────►│  (RAG & Semantic Match) │   │
│   └────────────────┘      └───────┬────────┘      └─────────────────────────┘   │
│                                   │                                             │
│               ┌───────────────────┼───────────────────┐                         │
│               ▼                   ▼                   ▼                         │
│       ┌───────────────┐   ┌───────────────┐   ┌───────────────┐                 │
│       │  team-domain  │   │  team-search  │   │  team-order   │                 │
│       │    (:50051)   │   │    (:50052)   │   │    (:50055)   │  ... (7 SVCS)   │
│       └───────┬───────┘   └───────▲───────┘   └───────┬───────┘                 │
│               │                   │                   │                         │
│               ▼ (Async Events)    │ (Consume & Index) ▼                         │
│       ┌───────────────────────────┴───────────────────────────┐                 │
│       │          Kafka Cluster / Redpanda (listing.*.events)  │                 │
│       └───────────────────────────────────────────────────────┘                 │
│                                                                                 │
│   ┌─────────────────────────────────────────────────────────────────────────┐   │
│   │                         OBSERVABILITY STACK                             │   │
│   │   OTel Collector  ──►  Prometheus (:9090)  ──►  Grafana Dashboard (:3001)│   │
│   │                   ──►  Jaeger Tracing (:16686)                          │   │
│   └─────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 🖥️ 2. MÔI TRƯỜNG LOCAL DEVELOPMENT (DOCKER COMPOSE)

Môi trường Local được đóng gói đầy đủ trong `platform-core/infra/docker-compose.yaml` để bất kỳ kỹ sư nào cũng có thể khởi chạy toàn bộ hạ tầng chỉ với 1 câu lệnh `docker compose -p platform-core up -d`:

| Service | Image | Cổng Host | Trách nhiệm |
|---|---|---|---|
| **`kafka-ui`** | `provectuslabs/kafka-ui:v0.7.2` | `:8088` | Giao diện web trực quan để soi topic, message payload và consumer lag |
| **`jaeger`** | `jaegertracing/all-in-one:1.57` | `:16686` | Web UI xem Distributed Tracing xuyên suốt Gateway $\rightarrow$ Services $\rightarrow$ Kafka |
| **`prometheus`** | `prom/prometheus:v2.52.0` | `:9090` | Thu thập time-series metrics từ OTel Collector & Redpanda |
| **`grafana`** | `grafana/grafana:11.0.0` | `:3001` | Dashboard trực quan hóa RPS, Latency p95/p99, Error rate, JVM/Go runtime |
| **`otel-collector`** | `otel/opentelemetry-collector-contrib:0.104.0` | `:4317, :4318` | Ingestion collector tiếp nhận OTLP Traces & Metrics từ tất cả services |
| **`redpanda`** | `redpandadata/redpanda:v24.1.7` | `:19092, :9644` | Kafka-compatible event streaming broker |
| **`rabbitmq`** | `rabbitmq:3-management` | `:5672, :15672` | Task queue broker (cho background jobs) |
| **`opensearch`** | `opensearchproject/opensearch:2.15.0` | `:9200` | Search read-model engine |
| **`minio`** | `minio/minio` | `:9000, :9001` | S3-compatible Object Storage lưu ảnh sản phẩm |
| **`qdrant`** | `qdrant/qdrant:v1.9.0` | `:6333, :6334` | Vector Database cho tìm kiếm tương đồng AI |
| **`redis`** | `redis:7-alpine` | `:6379` | L2 Distributed Cache & Session Store |
| **7 PostgreSQL DBs** | `postgres:16-alpine` | `:5433 - :5439` | Database-per-service độc lập cho 7 microservices |

---

## ☸️ 3. MÔ HÌNH TRIỂN KHAI PRODUCTION (KUBERNETES BLUEPRINT)

### A. Node Pools & Phân Nhóm Tài Nguyên:
1. **System Node Pool** (3x `c6i.xlarge`): Chạy Ingress Controller, CoreDNS, OTel Collector, Prometheus.
2. **Stateless Services Node Pool** (Auto-scale 5-50x `c6i.2xlarge`): Chạy `team-gateway`, `team-frontend`, 7 Go Microservices với **Horizontal Pod Autoscaler (HPA)** dựa trên CPU/Memory & gRPC Request Rate.
3. **Stateful Data Node Pool** (Dedicated `r6i.2xlarge` với EBS gp3 / NVMe): Chạy Managed Cloud Services (AWS RDS Multi-AZ Postgres, AWS OpenSearch Service, AWS MSK Kafka, AWS ElastiCache Redis).

### B. Mẫu Kubernetes Deployment Chuẩn Cho Go Microservice:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: team-domain-svc
  namespace: marketplace
  labels:
    app: team-domain-svc
spec:
  replicas: 3
  selector:
    matchLabels:
      app: team-domain-svc
  template:
    metadata:
      labels:
        app: team-domain-svc
    spec:
      containers:
        - name: service
          image: 123456789.dkr.ecr.ap-southeast-1.amazonaws.com/team-domain:v1.0.0
          ports:
            - containerPort: 50051
              name: grpc
          envFrom:
            - configMapRef:
                name: team-domain-config
            - secretRef:
                name: team-domain-secrets
          resources:
            requests:
              cpu: 250m
              memory: 256Mi
            limits:
              cpu: 1000m
              memory: 512Mi
          livenessProbe:
            grpc:
              port: 50051
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            grpc:
              port: 50051
            initialDelaySeconds: 3
            periodSeconds: 5
```
