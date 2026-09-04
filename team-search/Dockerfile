# syntax=docker/dockerfile:1
FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/indexer ./cmd/indexer

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/server /server
COPY --from=build /out/indexer /indexer
ENV GRPC_PORT=50052
EXPOSE 50052
USER nonroot:nonroot
ENTRYPOINT ["/server"]
