# team-audit — append-only audit trail service.
# Generated code is produced by `make proto` (buf) and is gitignored, not committed:
# run it before build/test. Never hand-edit anything under generated/.

.PHONY: proto build test tidy run

proto:
	buf generate

build:
	go build ./...

test:
	go test ./...

tidy:
	go mod tidy

run:
	go run ./cmd/server
