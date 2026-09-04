## platform-core — foundation tooling. Local-only, no cloud, no CI/CD (Phase 0).
## One fixed stack. proto is the single source of truth.

PROTO_DIR := packages/proto
COMPOSE   := docker compose -f infra/docker-compose.yaml

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: tools
tools: ## Verify required tooling is installed
	@command -v buf >/dev/null 2>&1 || { \
	  echo "✗ buf not installed. Install: brew install bufbuild/buf/buf"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "✗ docker not found"; exit 1; }
	@echo "✓ tooling ok ($$(buf --version), docker present)"

.PHONY: proto
proto: tools ## Lint + breaking-check + generate code into ./gen (all languages)
	cd $(PROTO_DIR) && buf lint
	@$(MAKE) breaking
	cd $(PROTO_DIR) && buf generate
	@echo "✓ generated into ./gen (go, python, jvm, ts)"

.PHONY: lint-proto
lint-proto: tools ## Lint proto only
	cd $(PROTO_DIR) && buf lint

.PHONY: breaking
breaking: tools ## Fail on breaking changes vs committed HEAD (no-op before first commit)
	@if git rev-parse HEAD >/dev/null 2>&1; then \
	  cd $(PROTO_DIR) && buf breaking --against '.git#ref=HEAD,subdir=packages/proto' \
	    && echo "✓ no breaking changes"; \
	else \
	  echo "· no git HEAD yet — skipping breaking-check (first commit)"; \
	fi

.PHONY: check
check: lint-proto breaking ## Merge-gate for platform-core (proto lint + breaking)
	@echo "✓ platform-core check passed"

.PHONY: dev
dev: ## Start local shared infra (postgres-*, redis, qdrant, redpanda, rabbitmq, opensearch)
	$(COMPOSE) up -d
	@echo "✅ infra up"
	@echo "  redis            localhost:6379"
	@echo "  qdrant           localhost:6333 (http) / 6334 (grpc)"
	@echo "  postgres-listing localhost:5433"
	@echo "  postgres-search  localhost:5434"
	@echo "  postgres-identity localhost:5435"
	@echo "  postgres-engagement localhost:5436"
	@echo "  redpanda (kafka) localhost:19092 (host) / redpanda:9092 (in-network); admin 9644"
	@echo "  rabbitmq         localhost:5672 (amqp) / 15672 (ui)"
	@echo "  opensearch       localhost:9200"
	@echo "  (observability profile off by default — see docs/ADR/0004)"

.PHONY: dev-obs
dev-obs: ## Start infra + observability profile (otel-collector)
	$(COMPOSE) --profile observability up -d

.PHONY: stop
stop: ## Stop infra (keep volumes)
	$(COMPOSE) down

.PHONY: clean
clean: ## Stop infra and delete volumes (destroys local data)
	$(COMPOSE) down -v

.PHONY: hooks-install
hooks-install: ## Point git at .githooks (enables pre-push merge-gate)
	git config core.hooksPath .githooks
	@echo "✓ git hooksPath -> .githooks"
