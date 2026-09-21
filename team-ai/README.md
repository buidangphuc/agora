# team-ai — Marketplace AI Engine (FastAPI & gRPC RAG Microservice)

`team-ai` is the central real-time **Generative AI and Intelligent Inference service** for the Agora marketplace. Running on **Python 3.12** with dual-transport interfaces (FastAPI HTTP on `:8000` and Connect-compatible gRPC on `:50050`), it provides buyers and sellers with low-latency LLM completions, context-aware Retrieval-Augmented Generation (RAG), automated product listing generation, and seller communication copilot capabilities.

---

## 1. System Design & Architecture Overview

`team-ai` delivers domain-tailored AI capabilities across the marketplace by bridging user interactions with upstream dense retrieval vector stores and LLM runtime clusters:
- **Buyer Experience**: Intelligent conversational Shopping Assistant powered by hybrid RAG that parses user queries, searches product embeddings in Qdrant, and returns structured product cards with follow-up suggestions.
- **Seller Operations**:
  - **Magic Listing Generator**: Automatically creates SEO-optimized product titles, detailed Markdown descriptions, category taxonomy tags, and price boundaries from sparse hints or images.
  - **Chat Copilot**: Real-time intent classification for customer inquiries providing 1-click contextual quick replies for sellers.
  - **Review Summarizer**: Multi-review sentiment distillation and pros/cons clustering for listings.
- **Dual Transport Integration**: Exposes both RESTful HTTP endpoints for direct frontend/AI testing and high-performance gRPC (`platform.ai.v1.AIService`) for edge routing via `team-gateway`.

---

## 2. Tech Stack

- **Application Framework**: Python 3.12, FastAPI, Uvicorn (ASGI), gRPC (`grpcio`, `grpcio-tools`, `grpc.aio`)
- **AI Orchestration & Knowledge Retrieval**:
  - **LangChain & LlamaIndex**: Prompt templating, agent chains, document ingestion, and context retrieval
  - **Pydantic v2**: Strict schema validation and serialization
- **Inference & Vector Backends**:
  - **platform-modelserve (`:8100`)**: Upstream gateway for Hugging Face TEI embeddings/rerankers and vLLM text generation
  - **Qdrant (`:6333`)**: High-performance approximate nearest neighbor (ANN) vector database
- **Data & Caching**: PostgreSQL (SQLAlchemy / Alembic), Redis (token buckets & rate limits)
- **Observability & MLOps**: Langfuse (LLM trace observability, token usage, latency), OpenTelemetry metrics & distributed tracing, Loguru structured logging

---

## 3. Architecture Diagram

```mermaid
flowchart TD
    subgraph Clients["Client Layer"]
        GW["team-gateway (:8080)"]
        FE["team-frontend (Next.js SSR)"]
    end

    subgraph TeamAI["team-ai Microservice (:8000 HTTP / :50050 gRPC)"]
        subgraph TransportLayer["Dual Transport Ingress"]
            GRPC["gRPC AIServicer\n(platform.ai.v1.AIService)"]
            FASTAPI["FastAPI REST Router\n(/api/v1/ai/*)"]
        end

        subgraph CoreEngine["AIAssistant Core Engine"]
            ASSISTANT["Shopping Assistant\n(RAG Pipeline)"]
            MAGIC["Magic Listing Generator\n(SEO & Categorizer)"]
            COPILOT["Chat Copilot\n(Intent Classifier & Quick Replies)"]
            SUMMARIZER["Review Summarizer\n(Sentiment & Pros/Cons)"]
        end

        subgraph AIOrchestration["AI / RAG Framework"]
            LC["LangChain / LlamaIndex\n(Prompt Chains & Vector Tools)"]
            EVAL["Langfuse Tracker\n(Traces, Tokens & Latency)"]
        end
    end

    subgraph ModelServing["Internal ML & Vector Infrastructure"]
        MS["platform-modelserve (:8100 Router)"]
        TEI_EMB["TEI Embeddings (:8101)"]
        TEI_RERANK["TEI Reranker (:8102)"]
        VLLM["vLLM Text Generation (:8103)"]
        QDRANT[("Qdrant Vector DB (:6333)")]
    end

    FE -->|Connect / REST| GW
    GW -->|gRPC x-principal-*| GRPC
    FE -.->|Direct HTTP (Dev/Docs)| FASTAPI

    GRPC --> ASSISTANT
    GRPC --> MAGIC
    GRPC --> COPILOT
    GRPC --> SUMMARIZER

    FASTAPI --> ASSISTANT
    FASTAPI --> MAGIC
    FASTAPI --> COPILOT
    FASTAPI --> SUMMARIZER

    ASSISTANT --> LC
    MAGIC --> LC
    COPILOT --> LC
    SUMMARIZER --> LC
    LC -.-> EVAL

    LC -->|Dense Vector Query| QDRANT
    LC -->|Embedding & Chat Completion| MS
    MS --> TEI_EMB
    MS --> TEI_RERANK
    MS --> VLLM
```

---

## 4. Internal Architecture & Data Flow

### A. RAG Shopping Assistant (`POST /api/v1/ai/assistant` / `ShoppingAssistant` gRPC)
1. **Query Ingestion**: Parses buyer natural language requirements (e.g., *"Find oversized 100% cotton t-shirt under 200k VND"*).
2. **Dense Vector & Semantic Search**: Queries `platform-modelserve` to embed the query text, fetches top-K listing candidates from Qdrant (`item_als_vectors` / catalog vectors), and cross-scores them with semantic keywords.
3. **Synthesis & Card Formulation**: Injects catalog context into LangChain prompts, synthesizing a personalized recommendation response along with structured `ProductCard` snippets (title, price, image, ratings, discount) and dynamic follow-up exploration prompts.

### B. Magic Listing Generator (`POST /api/v1/ai/magic-listing` / `MagicListing` gRPC)
1. **Input Normalization**: Ingests title hints, category clues, or product image URLs.
2. **SEO Optimization & Markdown Structuring**: Generates high-converting, keyword-dense titles, formatted Markdown product descriptions (Key Features, Specifications, Care Guidelines), and extracts trending search hashtags.
3. **Taxonomy & Price Guardrails**: Classifies the item into standard category taxonomies (`cat-electronics`, `cat-fashion`, etc.) and recommends calibrated competitive price bounds (`suggested_price_min`, `suggested_price_max`).

### C. Chat Copilot Smart Replies (`POST /api/v1/ai/chat-copilot` / `ChatCopilot` gRPC)
1. **Intent Extraction**: Analyzes buyer messages inside active buyer-seller chat threads.
2. **Classification Archetypes**: Identifies critical ecommerce customer intents:
   - Stock & Size Availability
   - Shipping & Delivery Timeline
   - Discounts, Vouchers & Best Offers
   - Return & Warranty Policy
3. **1-Click Suggestion Generation**: Generates 3 polite, accurate, and context-aware response variations for instant seller dispatch.

### D. Multi-Review Summarization (`SummarizeReviews` gRPC)
1. **Aggregated Review Ingestion**: Ingests batched customer reviews (star ratings and textual feedback).
2. **Aspect-Based Sentiment Clustering**: Distills collective sentiment, clusters prominent strengths (**Pros**) and recurring defects or complaints (**Cons**), and outputs an executive bulleted summary for product detail pages.

### E. Gateway & Edge Security Integration
- All downstream gRPC calls from `team-gateway` authenticate the user once and forward trusted identity via `x-principal-id`, `x-principal-type`, and `x-principal-scopes` metadata headers.
- Anonymous and authenticated users are routed seamlessly with rate-limiting and circuit-breaking managed at the edge.

---

## 5. Directory Structure

```text
app/
  api/
    v1/
      ai/                      # AI REST API routes (assistant, magic-listing, chat-copilot)
        dependencies.py        # Dependency injection for AIAssistantService
        router.py              # FastAPI APIRouter
      completions/             # Streaming and synchronous LLM completion endpoints
      health/                  # Liveness and readiness endpoints
  bootstrap/                   # App factory, lifecycle hooks, and gRPC background thread
  core/                        # Configuration, database engine, logging, OpenTelemetry
  modules/
    ai/
      llm/                     # LangChain Chat Model & Langfuse Tracker integrations
      rag/                     # LlamaIndex knowledge retrieval & vector tools
    business/
      ai_assistant/            # Core business logic: schemas, prompts, and service rules
        schemas.py             # Pydantic schemas (ProductCard, MagicListing, ChatCopilot, Reviews)
        service.py             # AIAssistantService implementation
  transport/
    grpc/                      # gRPC server, servicers (AIServicer), and generated proto stubs
proto/                         # Vendored platform contracts (platform.ai.v1)
```

---

## 6. Environment Configuration

| Variable | Default | Description |
|---|---|---|
| `HOST` | `0.0.0.0` | Bind address for FastAPI |
| `PORT` | `8000` | HTTP port for REST endpoints |
| `GRPC_PORT` | `50050` | gRPC server listening port |
| `MODELSERVE_URL` | `http://localhost:8100` | Address of platform-modelserve router |
| `QDRANT_URL` | `http://localhost:6333` | Vector database endpoint |
| `CHAT_MODEL` | `openai:gpt-4.1-mini` | LLM model identifier for LangChain completions |
| `LANGFUSE_ENABLED` | `false` | Enable Langfuse tracing and observability |
| `LANGFUSE_PUBLIC_KEY` | `""` | Langfuse public API key |
| `LANGFUSE_SECRET_KEY` | `""` | Langfuse secret API key |

---

## 7. Local Development & Testing

```bash
# 1. Setup environment with uv
cp .env.example .env
uv sync --dev

# 2. Run unit and integration tests
make test

# 3. Start development server with auto-reload
make dev
# REST Swagger Docs: http://localhost:8000/docs
# Health probe: http://localhost:8000/healthz
```
