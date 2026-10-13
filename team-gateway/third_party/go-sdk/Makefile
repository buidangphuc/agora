.PHONY: test lint check

# testcontainers reads DOCKER_HOST, and some runtimes (OrbStack) do not listen on
# /var/run/docker.sock, so take it from the active Docker context when unset.
test:
	@h="$${DOCKER_HOST:-$$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null)}"; \
	if [ -n "$$h" ]; then export DOCKER_HOST="$$h"; fi; \
	go test -race ./...

lint:
	go tool golangci-lint run ./...

# check never skips the Docker tests silently: without Docker they fail.
check:
	go vet ./...
	$(MAKE) lint
	GO_SDK_REQUIRE_DOCKER=1 $(MAKE) test
