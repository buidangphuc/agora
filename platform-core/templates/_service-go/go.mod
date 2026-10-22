// TEMPLATE — rename this module before first commit.
//
// Pick the real repo path for your service, e.g.
//   github.com/your-org/listing-service
// and replace every occurrence of `github.com/your-org/team-service`
// across this repo (go.mod, imports, buf.gen.yaml go_package_prefix).
//
// The SDK is required at a published tag; no replace and no vendored copy.
module github.com/your-org/team-service

go 1.26.0

require (
	github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.1
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)
