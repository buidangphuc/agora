# Local dev + CI merge-gate for the service. `make check` is the gate.
.DEFAULT_GOAL := help

# Path to the vendored proto module inside the submodule (see proto-vendor/README.md).
PROTO_SRC := proto-vendor/platform-core/packages/proto

.PHONY: help
help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: proto
proto: ## Generate Python/grpc stubs from vendored proto into ./generated
	@test -d "$(PROTO_SRC)" || { \
		echo "ERROR: $(PROTO_SRC) missing — run: git submodule update --init --recursive"; \
		exit 1; }
	buf generate "$(PROTO_SRC)"
	# Generated modules land under a top-level `platform` package and import each
	# other absolutely, so `generated/` goes on sys.path (see src/server.py). That
	# top-level `platform` SHADOWS the stdlib `platform` module. To harden against
	# that, generate with relative imports instead — see `make proto-relative`.

# ── Optional: proto with relative-import hardening (protoletariat) ────────────
# OPTIONAL variant. Runs protoletariat after `buf generate` to rewrite the
# generated absolute imports (`from platform.x.v1 ...`) into relative ones, so
# the generated tree can be imported as a private `generated` package WITHOUT a
# top-level `platform` on sys.path (which shadows stdlib `platform`). If you use
# this, import stubs as `from generated.platform.search.v1 import search_pb2`
# and drop the sys.path insert in src/server.py. Needs the `protoletariat` dev
# dep (`uv sync --extra dev`). The default `make proto` above stays the seed path.
.PHONY: proto-relative
proto-relative: ## Generate stubs + rewrite to relative imports (protoletariat)
	@test -d "$(PROTO_SRC)" || { \
		echo "ERROR: $(PROTO_SRC) missing — run: git submodule update --init --recursive"; \
		exit 1; }
	buf generate "$(PROTO_SRC)"
	uv run protol --create-package --in-place --python-out generated buf --config-path buf.gen.yaml

.PHONY: run
run: ## Run the gRPC server (needs `make proto` for the search RPC)
	uv run python src/main.py

.PHONY: test
test: ## Run the test suite
	uv run pytest

.PHONY: check
check: ## Merge-gate: lint + format + types + tests
	uv run ruff check .
	uv run ruff format --check .
	uv run pyright
	uv run pytest

# ── Optional: database migrations (alembic) ──────────────────────────────────
# Uncomment once a Postgres-backed capability (DATABASE_URL) is wired up.
# .PHONY: migrate
# migrate: ## Apply database migrations to head
# 	uv run alembic upgrade head
