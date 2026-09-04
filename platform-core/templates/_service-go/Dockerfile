# syntax=docker/dockerfile:1

# ── Stage 1: build a static binary ──────────────────────────────────────────
FROM golang:1.22 AS build
WORKDIR /src

# Cache modules first. NOTE: `make proto` must have produced ./generated before
# building the image (generated code is gitignored, so generate in CI/locally
# and either commit for the image build or run codegen as a prior build step).
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# CGO off => a fully static binary that runs on a distroless/scratch base.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ── Stage 2: minimal, non-root runtime ──────────────────────────────────────
# distroless/static:nonroot ships no shell and runs as an unprivileged user.
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/server /server

# Documented default; the process reads GRPC_PORT at runtime.
ENV GRPC_PORT=50051
EXPOSE 50051

USER nonroot:nonroot
ENTRYPOINT ["/server"]
