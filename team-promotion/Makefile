ifneq (,$(wildcard ./.env))
include .env
export
endif
DATABASE_URL ?= postgres://promotion_svc:promotion_pass@localhost:5438/promotion_db?sslmode=disable
.PHONY: help proto run check check-env test migrate
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
proto: ## Generate Go code from the vendored proto module (needs buf)
	buf generate
run: ## Run the promotion gRPC server
	go run ./cmd/server
check: check-env ## Merge gate: env-drift, gofmt, vet, tests
	@echo ">> gofmt (report unformatted files)"
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "gofmt needs: $$unformatted"; exit 1; fi
	@echo ">> go vet" ; go vet ./...
	@echo ">> go test" ; go test ./...
check-env: ## Verify .env.example matches internal/config
	go test ./internal/config/ -run TestDeclaredEnvKeysNoDrift -count=1
test: ## Run all tests
	go test ./...
migrate: ## Apply DB migrations (golang-migrate via docker)
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate \
	  -path=/migrations -database "$(subst localhost,host.docker.internal,$(DATABASE_URL))" up
