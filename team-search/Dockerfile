# syntax=docker/dockerfile:1
# Go service image (ADR-0016). Both binaries ship in one image: /server is the
# gRPC query API (the default entrypoint) and /indexer is the Kafka indexer
# (deploys override the entrypoint). Build: docker build -t team-search:dev .
# The build context is the service dir; third_party/go-sdk must be synced first
# and generated/ must exist (not committed): `make proto` creates it.

FROM golang:1.26 AS build
WORKDIR /src

# go.mod replaces the SDK with ./third_party/go-sdk, so it must be present
# before `go mod download`.
COPY go.mod go.sum ./
COPY third_party/ third_party/
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/indexer ./cmd/indexer

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/server /server
COPY --from=build /out/indexer /indexer

ARG GRPC_PORT=50052
ENV GRPC_PORT=${GRPC_PORT}
EXPOSE ${GRPC_PORT}

USER nonroot:nonroot
ENTRYPOINT ["/server"]
