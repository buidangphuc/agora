# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BIN=server
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${BIN}

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/app /app

ENV GRPC_PORT=50051
EXPOSE 50051

USER nonroot:nonroot
ENTRYPOINT ["/app"]
