.PHONY: test lint check

test:
	go test -race ./...

lint:
	go tool golangci-lint run ./...

check:
	go vet ./...
	$(MAKE) lint
	$(MAKE) test
