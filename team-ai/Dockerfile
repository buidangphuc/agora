# syntax=docker/dockerfile:1.7

# Build stage: resolve deps into an isolated virtualenv
FROM python:3.12-slim AS builder

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_CACHE_DIR=/tmp/uv-cache \
    UV_LINK_MODE=copy \
    UV_PROJECT_ENVIRONMENT=/app/.venv

WORKDIR /app

COPY --from=ghcr.io/astral-sh/uv:0.5.4 /uv /usr/local/bin/uv
COPY . .
# Opt-in optional extras (e.g. --build-arg UV_EXTRAS=ai, or "ai aws").
# Defaults to empty so the minimal-core image stays small.
ARG UV_EXTRAS=""
RUN --mount=type=cache,target=/tmp/uv-cache \
    uv sync --no-dev \
    $(for e in $UV_EXTRAS; do printf -- '--extra %s ' "$e"; done)

# Proto stage: the gRPC stubs under app/transport/grpc/_pb are generated (git-ignored),
# so a clean checkout has none. Generate them from the vendored proto/ contract with the
# same grpcio-tools version pinned in uv.lock; kept out of the app venv.
FROM python:3.12-slim AS protogen
WORKDIR /src
COPY --from=ghcr.io/astral-sh/uv:0.5.4 /uv /usr/local/bin/uv
RUN --mount=type=cache,target=/tmp/uv-cache UV_CACHE_DIR=/tmp/uv-cache UV_HTTP_TIMEOUT=300 \
    uv pip install --system "grpcio-tools==1.71.2"
COPY proto ./proto
COPY scripts/gen_proto.py ./scripts/gen_proto.py
RUN python -m scripts.gen_proto

# Runtime stage: minimal image, non-root, healthcheck
FROM python:3.12-slim AS runtime

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/app/.venv/bin:${PATH}"

RUN groupadd --system --gid 1001 app \
    && useradd --system --uid 1001 --gid app --home-dir /app --shell /sbin/nologin app

WORKDIR /app

COPY --from=builder --chown=app:app /app/.venv /app/.venv
COPY --chown=app:app app ./app
COPY --from=protogen --chown=app:app /src/app/transport/grpc/_pb ./app/transport/grpc/_pb
COPY --chown=app:app alembic ./alembic
COPY --chown=app:app main.py alembic.ini ./

USER app

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD python -c "import urllib.request, sys; sys.exit(0 if urllib.request.urlopen('http://localhost:8000/healthz', timeout=3).status == 200 else 1)"

CMD ["uvicorn", "main:app", \
     "--host", "0.0.0.0", \
     "--port", "8000", \
     "--loop", "uvloop", \
     "--http", "httptools", \
     "--no-access-log"]
