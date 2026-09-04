.PHONY: proto build test run tidy

# Regenerate Go stubs from the vendored proto module (buf cache).
proto:
	buf generate

build:
	go build ./...

test:
	go test ./...

run:
	go run ./cmd/server

tidy:
	go mod tidy
