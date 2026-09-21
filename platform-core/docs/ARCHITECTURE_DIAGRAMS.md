# 🏛️ HỆ THỐNG KIẾN TRÚC TOÀN DIỆN AGORA MARKETPLACE (SYSTEM & PER-APP DIAGRAMS)

> Bộ thiết kế sơ đồ kiến trúc chuẩn doanh nghiệp (Enterprise-Grade Architecture Diagrams) cho nền tảng **Agora Polyrepo AI-First Marketplace**, lấy cảm hứng từ các pipeline MLOps & E-commerce phân tán hiện đại.

---

## 📑 MỤC LỤC SƠ ĐỒ

1. [🌐 SƠ ĐỒ TỔNG THỂ TOÀN BỘ HỆ THỐNG (ALL-IN-ONE ARCHITECTURE)](#1-sơ-đồ-tổng-thể-toàn-bộ-hệ-thống-all-in-one-architecture)
2. [🚪 EDGE GATEWAY & BẢO MẬT ZERO-TRUST (CONNECT EDGE + RS256 JWKS)](#2-edge-gateway--bảo-mật-zero-trust-connect-edge--rs256-jwks)
3. [⚡ PIPELINE DỮ LIỆU & TELEMETRY (GA4 DATALAYER + CQRS + KAFKA + WAREHOUSE)](#3-pipeline-dữ-liệu--telemetry-ga4-datalayer--cqrs--kafka--warehouse)
4. [🧠 MLOPS & PIPELINE HUẤN LUYỆN / PHỤC VỤ MÔ HÌNH (RECSYS + FORECASTING + MODELSERVE)](#4-mlops--pipeline-huấn-luyện--phục-vụ-mô-hình-recsys--forecasting--modelserve)
5. [🔄 FEATURE STORE & ONLINE/OFFLINE FEATURE SERVING](#5-feature-store--onlineoffline-feature-serving)
6. [💳 DISTRIBUTED SAGA & TRANSACTIONAL OUTBOX (ORDER + PROMOTION + PAYMENT)](#6-distributed-saga--transactional-outbox-order--promotion--payment)
7. [🔍 HYBRID SEARCH RETRIEVAL & VECTOR SEARCH ENGINE](#7-hybrid-search-retrieval--vector-search-engine)
8. [📊 OBSERVABILITY & GITOPS K8S CLUSTER DEPLOYMENT](#8-observability--gitops-k8s-cluster-deployment)

---

## 1. 🌐 SƠ ĐỒ TỔNG THỂ TOÀN BỘ HỆ THỐNG (ALL-IN-ONE ARCHITECTURE)

Sơ đồ thể hiện toàn bộ các tầng: **Client/Browser** $\rightarrow$ **Edge Gateway** $\rightarrow$ **Core Business Microservices** $\rightarrow$ **Event Streaming & CDC** $\rightarrow$ **ML & Analytics Platform** $\rightarrow$ **Database per Service**.

```mermaid
flowchart TB
    %% STYLES
    classDef client fill:#3b82f6,stroke:#1d4ed8,stroke-width:2px,color:#fff;
    classDef gateway fill:#8b5cf6,stroke:#6d28d9,stroke-width:2px,color:#fff;
    classDef service fill:#10b981,stroke:#047857,stroke-width:2px,color:#fff;
    classDef data fill:#f59e0b,stroke:#b45309,stroke-width:2px,color:#fff;
    classDef ml fill:#ec4899,stroke:#be185d,stroke-width:2px,color:#fff;
    classDef infra fill:#64748b,stroke:#334155,stroke-width:2px,color:#fff;

    subgraph Layer_Clients["📱 CLIENT & APPLICATION LAYER"]
        Web["Web SSR (Next.js 14 App Router)<br/>:3000"]:::client
        Mobile["Mobile / PWA Client"]:::client
        DataLayer["GA4 DataLayer & Telemetry Client"]:::client
    end

    subgraph Layer_Edge["🛡️ EDGE GATEWAY & SECURITY LAYER"]
        Gateway["team-gateway (Connect Edge)<br/>:8080<br/>• RS256 JWT Verify (Verify-only)<br/>• Rate Limit & CORS<br/>• Upstream Deadlines & Retries"]:::gateway
    end

    subgraph Layer_Services["⚙️ CORE BUSINESS MICROSERVICES (gRPC / DB-Per-Service)"]
        Identity["team-identity (:50053)<br/>Auth, RS256 Sign, JWKS (:50063)"]:::service
        Domain["team-domain (:50051)<br/>Listing Write-Model & Outbox"]:::service
        Order["team-order (:50055)<br/>Saga Orchestrator, Cart, RMA"]:::service
        Payment["team-payment (:50056)<br/>Mock Wallet, Ledger, Payout"]:::service
        Promotion["team-promotion (:50061)<br/>Vouchers, Flash Sales"]:::service
        Engagement["team-engagement (:50054)<br/>Reviews, Favorites, Q&A"]:::service
        Chat["team-chat (:50057)<br/>Real-time Chat Threads"]:::service
        Notification["team-notification (:50058)<br/>In-app Alerts & Push"]:::service
        Verification["team-verification (:50064)<br/>KYC Submissions"]:::service
        Sharing["team-sharing (:50065)<br/>Short Links & Attribution"]:::service
        Audit["team-audit (:50066)<br/>Immutable Audit Log"]:::service
    end

    subgraph Layer_Streaming["📨 EVENT BROKER & CDC (KAFKA / REDPANDA)"]
        Kafka_Listing["Kafka: listing.events"]:::data
        Kafka_Order["Kafka: order.events"]:::data
        Kafka_Analytics["Kafka: analytics.events"]:::data
        Kafka_Promotion["Kafka: promotion.events"]:::data
    end

    subgraph Layer_Search_Analytics["🔎 SEARCH READ-MODEL & WAREHOUSE"]
        Search["team-search (:50052)<br/>Hybrid BM25 + Vector Search"]:::service
        OpenSearch[("OpenSearch Index<br/>:9200")]:::data
        Analytics["team-analytics (:50059)<br/>DuckDB Analytics Warehouse"]:::service
        DuckDB[("DuckDB Analytics Facts")]:::data
    end

    subgraph Layer_MLOps["🧠 MACHINE LEARNING & RECSYS PLATFORM"]
        Recsys["platform-recsys<br/>• Offline ALS Matrix Factorization<br/>• Two-Tower Retrieval<br/>• GBDT CVR / eGMV Ranker"]:::ml
        Forecast["platform-forecast<br/>ADR-0013 Demand Forecasting<br/>(P10/P50/P90 Quantiles)"]:::ml
        ModelServe["platform-modelserve (:8100)<br/>ML Serving Router (TEI / vLLM)"]:::ml
        TeamAI["team-ai (:8000 / :50060)<br/>AI Copilot, RAG, Magic Listing"]:::ml
        Qdrant[("Qdrant Vector DB<br/>:6333")]:::data
        RedisFeature[("Redis / Valkey Feature Store<br/>:6379")]:::data
    end

    %% CONNECTIONS
    Web -->|HTTP / JSON / gRPC-web| Gateway
    Mobile -->|Connect / gRPC-web| Gateway
    DataLayer -.->|POST /api/track| Gateway

    Gateway -->|gRPC + Principal Header| Identity
    Gateway -->|gRPC + Principal Header| Domain
    Gateway -->|gRPC + Principal Header| Order
    Gateway -->|gRPC + Principal Header| Payment
    Gateway -->|gRPC + Principal Header| Promotion
    Gateway -->|gRPC + Principal Header| Engagement
    Gateway -->|gRPC + Principal Header| Chat
    Gateway -->|gRPC + Principal Header| Notification
    Gateway -->|gRPC + Principal Header| Search
    Gateway -->|gRPC + Principal Header| Analytics
    Gateway -->|gRPC + Principal Header| TeamAI
    Gateway -->|gRPC + Principal Header| Verification
    Gateway -->|gRPC + Principal Header| Sharing
    Gateway -->|gRPC + Principal Header| Audit

    Domain -->|Publish Outbox| Kafka_Listing
    Order -->|Publish Outbox| Kafka_Order
    Promotion -->|Publish Outbox| Kafka_Promotion
    Gateway -.->|Forward Beacons| Kafka_Analytics

    Kafka_Listing -->|Consume & Index| Search
    Search --> OpenSearch

    Kafka_Analytics -->|Consume & Aggregate| Analytics
    Kafka_Order -->|Consume Facts| Analytics
    Analytics --> DuckDB

    DuckDB -.->|Offline Training Data| Recsys
    DuckDB -.->|Historical Demand| Forecast

    Recsys -->|Publish Candidate Vectors| Qdrant
    Recsys -->|Precompute Top-N Cache| RedisFeature
    TeamAI -->|Dense Retrieval| Qdrant
    TeamAI -->|Online Features| RedisFeature
    TeamAI -->|Inference| ModelServe
```

---

## 2. 🚪 EDGE GATEWAY & BẢO MẬT ZERO-TRUST (CONNECT EDGE + RS256 JWKS)

Kiến trúc xác thực **một lần duy nhất tại Edge** (ADR-0003 & ADR-0006). Gateway không giữ private key, chỉ đọc public JWKS để xác minh token và forward `x-principal-*` đã được bảo chứng.

```mermaid
sequenceDiagram
    autonumber
    actor Client as Browser / Mobile App
    participant GW as team-gateway (:8080)
    participant ID as team-identity (:50053 / :50063)
    participant Svc as Downstream Services (Order, Listing...)

    Note over ID,GW: 1. Khởi động: Gateway nạp & cache JWKS từ Identity
    GW->>ID: GET /.well-known/jwks.json (:50063)
    ID-->>GW: JWKS (Public RSA Keys + Key IDs 'kid')

    Note over Client,ID: 2. Đăng nhập & Cấp phát JWT
    Client->>GW: POST /platform.identity.v1.AuthService/Login
    GW->>ID: Forward gRPC Login
    ID->>ID: Ký Token bằng JWT_PRIVATE_KEY (RS256)
    ID-->>GW: JWT Token (Bearer eyJhbGciOiJSUzI1NiIsImtpZCI6InYxIi...)
    GW-->>Client: Trả về Session Token

    Note over Client,Svc: 3. Gọi dịch vụ có xác thực
    Client->>GW: POST /platform.order.v1.OrderService/CreateOrder (Authorization: Bearer <JWT>)
    GW->>GW: Xác minh chữ ký số RS256 qua JWKS Key (khớp 'kid')
    GW->>GW: Trích xuất Claims: Principal{ID: "usr-123", Role: "buyer", Scopes: ["order:write"]}
    GW->>Svc: Forward gRPC Request + Metadata:<br/>x-principal-id: usr-123<br/>x-principal-type: user<br/>x-principal-scopes: order:write,listing.read
    Svc->>Svc: Interceptor: RequireScopes("order:write") -> Hợp lệ
    Svc-->>GW: gRPC Response
    GW-->>Client: JSON / Connect Response
```

---

## 3. ⚡ PIPELINE DỮ LIỆU & TELEMETRY (GA4 DATALAYER + CQRS + KAFKA + WAREHOUSE)

Pipeline dữ liệu hành vi người dùng, đảm bảo **Zero Data Pollution**, tối ưu Edge Bandwidth qua **Batched Impressions** và fan-out nhiều sản phẩm chung một `event_group_id`.

```mermaid
flowchart LR
    classDef client fill:#3b82f6,stroke:#1d4ed8,stroke-width:2px,color:#fff;
    classDef edge fill:#8b5cf6,stroke:#6d28d9,stroke-width:2px,color:#fff;
    classDef stream fill:#f59e0b,stroke:#b45309,stroke-width:2px,color:#fff;
    classDef lake fill:#10b981,stroke:#047857,stroke-width:2px,color:#fff;
    classDef app fill:#ec4899,stroke:#be185d,stroke-width:2px,color:#fff;

    subgraph Browser["🌐 Frontend Browser / UI"]
        DL["window.dataLayer.push({ ecommerce: null })<br/>(Reset state)"]:::client
        DL_Event["dataLayer.push({ event: 'view_item_list', items: [...] })"]:::client
        BeaconCollector["trackEcommerce() Dispatcher<br/>(Batched 1 HTTP Request)"]:::client
    end

    subgraph Edge["🚪 Edge Gateway"]
        TrackRoute["POST /api/track<br/>• Validate schema<br/>• Multi-Item Fan-out<br/>• Bind event_group_id"]:::edge
    end

    subgraph KafkaCluster["📨 Kafka / Redpanda Message Bus"]
        Topic_Analytics["Topic: analytics.events<br/>Key: session_id / anonymous_id<br/>Payload: platform.analytics.v1.TrackingEvent"]:::stream
        Topic_Domain["Topic: listing.events<br/>Key: listing_id<br/>Payload: ListingChangedEvent"]:::stream
        Topic_Order["Topic: order.events<br/>Key: order_id<br/>Payload: OrderPaidEvent"]:::stream
    end

    subgraph StorageLayer["💾 Storage & Read-Models"]
        AnalyticsWorker["team-analytics Consumer<br/>(:50059)"]:::app
        DuckDB[("DuckDB Parquet Warehouse<br/>• bronze_raw_events<br/>• silver_fact_orders<br/>• gold_seller_funnels")]:::lake
        SearchIndexer["team-search Consumer<br/>(:50052)"]:::app
        OpenSearch[("OpenSearch Read-Model<br/>listings_index")]:::lake
    end

    DL --> DL_Event --> BeaconCollector
    BeaconCollector -->|POST JSON Beacons| TrackRoute
    TrackRoute --> Topic_Analytics

    Topic_Analytics --> AnalyticsWorker
    Topic_Order --> AnalyticsWorker
    AnalyticsWorker --> DuckDB

    Topic_Domain --> SearchIndexer
    SearchIndexer --> OpenSearch
```

---

## 4. 🧠 MLOPS & PIPELINE HUẤN LUYỆN / PHỤC VỤ MÔ HÌNH (RECSYS + FORECASTING + MODELSERVE)

Vòng đời khép kín từ **Thu thập tương tác $\rightarrow$ Huấn luyện Offline ALS / Two-Tower $\rightarrow$ Đánh giá Temporal Holdout & Promotion Gate $\rightarrow$ Triển khai Serving**.

```mermaid
flowchart TD
    classDef data fill:#f59e0b,stroke:#b45309,stroke-width:2px,color:#fff;
    classDef train fill:#ec4899,stroke:#be185d,stroke-width:2px,color:#fff;
    classDef reg fill:#8b5cf6,stroke:#6d28d9,stroke-width:2px,color:#fff;
    classDef serve fill:#10b981,stroke:#047857,stroke-width:2px,color:#fff;

    subgraph DataPrep["1. Chuẩn bị Dữ liệu Huấn luyện"]
        DuckDB_Facts[("DuckDB Warehouse Facts<br/>Interactions (view, click, cart, purchase)")]:::data
        TemporalSplit["Temporal Split<br/>Train: Days 1-23 | Test: Days 24-30"]:::data
    end

    subgraph TrainingEngine["2. Huấn luyện Mô hình Đa nhiệm (platform-recsys)"]
        ALS_Train["Implicit ALS Matrix Factorization<br/>(Latent Factors: 64)"]:::train
        TwoTower_Train["Two-Tower DNN Embedding<br/>(User Tower + Item Tower)"]:::train
        GBDT_Ranker["LightGBM GBDT Ranker<br/>(CVR Prediction & eGMV Debiasing)"]:::train
    end

    subgraph EvaluationGate["3. Đánh giá Offline & Promotion Gate"]
        Evaluator["ModelEvaluator<br/>Calculate NDCG@10, MAP@10, Coverage"]:::reg
        PromotionGate{"Promotion Gate<br/>NDCG_candidate >= NDCG_champion - 0.005?"}:::reg
        Registry[("Model Registry Metadata<br/>Status: CHAMPION / REJECTED")]:::reg
    end

    subgraph DeploymentServing["4. Triển khai & Phục vụ Trực tuyến"]
        QdrantIndex[("Qdrant Vector DB<br/>Item Embeddings Collection")]:::serve
        RedisPrecompute[("Redis Top-N Cache<br/>recs:v1:user:{id}")]:::serve
        TeamAI_Serving["team-ai (:50060) / platform-modelserve (:8100)<br/>Recommend RPC / Two-Stage Retrieval"]:::serve
    end

    DuckDB_Facts --> TemporalSplit
    TemporalSplit --> ALS_Train & TwoTower_Train & GBDT_Ranker
    ALS_Train & TwoTower_Train & GBDT_Ranker --> Evaluator
    Evaluator --> PromotionGate

    PromotionGate -- Pass --> Registry
    PromotionGate -- Pass --> QdrantIndex
    PromotionGate -- Pass --> RedisPrecompute
    PromotionGate -- Fail --> Registry

    QdrantIndex --> TeamAI_Serving
    RedisPrecompute --> TeamAI_Serving
```

---

## 5. 🔄 FEATURE STORE & ONLINE/OFFLINE FEATURE SERVING

Kiến trúc **Feature Store** nhất quán giữa môi trường Online (thời gian thực) và Offline (huấn luyện hàng loạt), ngăn chặn hiện tượng Train-Serve Skew.

```mermaid
flowchart LR
    classDef src fill:#3b82f6,stroke:#1d4ed8,stroke-width:2px,color:#fff;
    classDef store fill:#f59e0b,stroke:#b45309,stroke-width:2px,color:#fff;
    classDef sync fill:#8b5cf6,stroke:#6d28d9,stroke-width:2px,color:#fff;
    classDef client fill:#10b981,stroke:#047857,stroke-width:2px,color:#fff;

    subgraph FeatureSources["Nguồn Đặc trưng (Raw Signals)"]
        StreamEvents["Kafka Real-time Streams<br/>(Cập nhật click, view 15m)"]:::src
        BatchDB["PostgreSQL / Warehouse<br/>(Doanh số 30d, Tỷ lệ hoàn hàng)"]:::src
    end

    subgraph FeatureStore["platform-featurestore"]
        Entity_User["Entity: User Features<br/>• user_30d_orders<br/>• user_avg_order_val<br/>• user_fav_category"]:::sync
        Entity_Item["Entity: Item Features<br/>• item_7d_views<br/>• item_ctr_score<br/>• item_stock_level"]:::sync
    end

    subgraph Storage["Kho Đặc trưng Hai Tầng"]
        OnlineStore[("Valkey / Redis Online Store<br/>Latency: < 2ms")]:::store
        OfflineStore[("PostgreSQL / Parquet Offline Store<br/>Batch SQL / Spark")]:::store
    end

    subgraph Consumers["Các Ứng dụng Tiêu thụ"]
        RankerServing["GBDT Ranker / ModelServe<br/>(Online Inference)"]:::client
        ModelTraining["Spark / Ray ML Training<br/>(Point-in-time Joins)"]:::client
    end

    StreamEvents -->|Nearline Sync| Entity_User & Entity_Item
    BatchDB -->|Daily Ingestion| Entity_User & Entity_Item

    Entity_User & Entity_Item -->|Materialize| OnlineStore
    Entity_User & Entity_Item -->|Append History| OfflineStore

    OnlineStore -->|GetOnlineFeatures()| RankerServing
    OfflineStore -->|GetHistoricalFeatures()| ModelTraining
```

---

## 6. 💳 DISTRIBUTED SAGA & TRANSACTIONAL OUTBOX (ORDER + PROMOTION + PAYMENT)

Mô hình giao dịch phân tán **Orchestrated Saga Pattern** kết hợp với **Transactional Outbox Pattern** đảm bảo tính toàn vẹn dữ liệu tài chính và thanh toán.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Người mua (Nguyễn Văn An)
    participant Edge as team-gateway
    participant Order as team-order (Saga Orchestrator)
    participant Promo as team-promotion
    participant Domain as team-domain (Stock Reserve)
    participant Pay as team-payment (Mock Wallet)
    participant Outbox as order_outbox_events
    participant Relayer as Outbox Relayer Worker
    participant Kafka as Kafka order.events

    Buyer->>Edge: Nhấn Đặt hàng (Voucher, SPX Express, Mock Wallet)
    Edge->>Order: CreateOrder(items, voucher_code, payment_method)

    rect rgb(240, 248, 255)
        Note over Order,Pay: SAGA STEP 1: Khóa Voucher Khuyến mãi
        Order->>Promo: ReserveVoucher(code: "WELCOME50", buyer_id)
        Promo-->>Order: VoucherReserved (-50.000 VND)

        Note over Order,Domain: SAGA STEP 2: Khóa Tồn kho Sản phẩm
        Order->>Domain: ReserveStock(listing_id, qty)
        Domain-->>Order: StockReserved (ReservationID: res-9981)

        Note over Order,Pay: SAGA STEP 3: Trừ tiền Ví / Ủy quyền thanh toán
        Order->>Pay: AuthorizePayment(amount: 31.179.000 VND)
        Pay-->>Order: PaymentCaptured (TxnID: txn-pay-4412)
    end

    rect rgb(245, 255, 245)
        Note over Order,Outbox: Giao dịch Nguyên tử DB: Cập nhật Order PAID + Ghi Outbox
        Order->>Order: BEGIN DB TRANSACTION
        Order->>Order: UPDATE orders SET status = 'PAID'
        Order->>Outbox: INSERT INTO order_outbox_events (event_type: 'OrderPaidEvent')
        Order->>Order: COMMIT DB TRANSACTION
    end

    Order-->>Edge: Order Success (Order ID: ord-891, Status: PAID)
    Edge-->>Buyer: Trả về màn hình Xác nhận Đơn hàng

    rect rgb(255, 250, 240)
        Note over Outbox,Kafka: Relayer quét Outbox & Đẩy sang Kafka
        Relayer->>Outbox: Quét các sự kiện 'PENDING'
        Relayer->>Kafka: Publish OrderPaidEvent (Key: ord-891)
        Relayer->>Outbox: UPDATE order_outbox_events SET status = 'PUBLISHED'
    end
```

---

## 7. 🔍 HYBRID SEARCH RETRIEVAL & VECTOR SEARCH ENGINE

Kiến trúc tìm kiếm lai kết hợp **Khớp từ khóa BM25** trên OpenSearch và **Tìm kiếm ngữ nghĩa Dense Vector (Qdrant)**, dung hợp kết quả bằng thuật toán **Reciprocal Rank Fusion (RRF)**.

```mermaid
flowchart TD
    classDef query fill:#3b82f6,stroke:#1d4ed8,stroke-width:2px,color:#fff;
    classDef engine fill:#8b5cf6,stroke:#6d28d9,stroke-width:2px,color:#fff;
    classDef fusion fill:#ec4899,stroke:#be185d,stroke-width:2px,color:#fff;
    classDef res fill:#10b981,stroke:#047857,stroke-width:2px,color:#fff;

    Query["User Query: 'iPhone 15 Pro Max 256GB'"]:::query

    subgraph Branch_Lexical["1. Nhánh Tìm kiếm Lexical (BM25)"]
        OpenSearch_Query["OpenSearch Query<br/>• Multi-match on Title & Description<br/>• Category filter & Price Range"]:::engine
        BM25_Results["BM25 Top-50 Candidates<br/>(Điểm số Lexical Score)"]:::engine
    end

    subgraph Branch_Semantic["2. Nhánh Tìm kiếm Semantic (Dense Vector)"]
        Embedder["Text Embedding Model<br/>(platform-modelserve / TEI :8101)"]:::engine
        Vector_Query["Qdrant Vector Search<br/>• Cosine Similarity search<br/>• HNSW Vector Index"]:::engine
        Vector_Results["Vector Top-50 Candidates<br/>(Điểm số Cosine Similarity)"]:::engine
    end

    subgraph FusionEngine["3. Dung hợp & Xếp hạng lại (RRF & GBDT Ranker)"]
        RRF["Reciprocal Rank Fusion (RRF)<br/>Score = Σ (1 / (60 + rank_i))"]:::fusion
        FilterStock["Freshness & Out-of-stock Filter"]:::fusion
        Ranker["LightGBM Ranker<br/>(Xếp hạng tối ưu theo CTR/eGMV)"]:::fusion
    end

    FinalList["Top-20 Sản phẩm Hiển thị trên Grid UI"]:::res

    Query --> OpenSearch_Query --> BM25_Results
    Query --> Embedder --> Vector_Query --> Vector_Results

    BM25_Results & Vector_Results --> RRF
    RRF --> FilterStock --> Ranker --> FinalList
```

---

## 8. 📊 OBSERVABILITY & GITOPS K8S CLUSTER DEPLOYMENT

Mô hình triển khai hạ tầng **Kubernetes GitOps (ArgoCD + Kind + Vault)** kết hợp hệ thống giám sát toàn diện chuẩn **OpenTelemetry / RED Metrics**.

```mermaid
flowchart TD
    classDef gitops fill:#3b82f6,stroke:#1d4ed8,stroke-width:2px,color:#fff;
    classDef k8s fill:#8b5cf6,stroke:#6d28d9,stroke-width:2px,color:#fff;
    classDef obs fill:#f59e0b,stroke:#b45309,stroke-width:2px,color:#fff;
    classDef sec fill:#10b981,stroke:#047857,stroke-width:2px,color:#fff;

    subgraph GitOps_Control["GitOps & Secret Management"]
        GitRepo["Git Repository (Gitea / GitHub)<br/>deploy/argocd/root-app.yaml"]:::gitops
        ArgoCD["ArgoCD Controller<br/>(Tự động đồng bộ K8s Manifests)"]:::gitops
        Vault["HashiCorp Vault<br/>(Quản lý JWT Private Keys & DB Passwords)"]:::sec
        ESO["External Secrets Operator (ESO)<br/>(Sync Secrets vào K8s Secrets)"]:::sec
    end

    subgraph K8s_Cluster["Kubernetes Cluster (Namespace: marketplace)"]
        IngressNginx["Ingress NGINX Controller<br/>(api.localtest.me / marketplace.localtest.me)"]:::k8s
        Pod_Gateway["team-gateway Pods"]:::k8s
        Pod_Services["Core Microservices Pods<br/>(Identity, Order, Domain, Search...)"]:::k8s
        Pod_Infra["In-cluster DBs & Brokers<br/>(PostgreSQL, Redpanda, Valkey, OpenSearch)"]:::k8s
    end

    subgraph Observability_Stack["Telemetry & Monitoring (RED Metrics / Traces)"]
        OTel["OpenTelemetry Collector (:4317)"]:::obs
        Prometheus["Prometheus (:9090)<br/>(Scrape Rate, Errors, Duration)"]:::obs
        Grafana["Grafana Dashboards (:3001)"]:::obs
        Jaeger["Jaeger Tracing / SigNoz<br/>(Distributed Trace Spans)"]:::obs
    end

    GitRepo --> ArgoCD --> IngressNginx & Pod_Gateway & Pod_Services
    Vault --> ESO --> Pod_Services

    IngressNginx --> Pod_Gateway
    Pod_Gateway --> Pod_Services
    Pod_Services --> Pod_Infra

    Pod_Gateway & Pod_Services -.->|OTLP gRPC| OTel
    OTel --> Prometheus --> Grafana
    OTel --> Jaeger
```

---

## 🎯 TÓM TẮT GIÁ TRỊ THIẾT KẾ

1. **Chuẩn Polyrepo Doanh nghiệp:** Tách biệt rõ ràng ranh giới giữa tầng Edge (`team-gateway`), tầng Nghiệp vụ (`team-*`), tầng Dữ liệu / AI (`platform-*`), và tầng Triển khai (`platform-e2e` / `deploy`).
2. **Không có Seam bị hở:** Mọi luồng từ JWT RS256/JWKS, Multi-Shop Cart, Distributed Saga, Outbox Relayer đến AI RecSys và Telemetry GA4 đều được ánh xạ 1:1 với codebase đang chạy thực tế.
3. **Dễ bảo trì & Mở rộng:** Dùng chuẩn Mermaid Markdown tương thích 100% với GitHub, GitLab, VS Code, Notion và các công cụ render tài liệu kỹ thuật cao cấp.
