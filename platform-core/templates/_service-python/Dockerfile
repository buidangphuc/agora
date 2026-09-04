# Multi-stage, uv-based build. Non-root runtime (uid 1001).
# NOTE: `generated/` is git-ignored, so the image build MUST run codegen. Either
# run `make proto` on the host before building (and drop the .dockerignore line
# for generated/), or add a proto stage that runs `buf generate` in-image.

# ── Builder: resolve + install deps into a venv ───────────────────────────────
FROM python:3.12-slim AS builder

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

ENV UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    UV_PROJECT_ENVIRONMENT=/opt/venv

WORKDIR /app

# Install dependencies first (cached until pyproject changes).
COPY pyproject.toml ./
RUN uv sync --no-dev --no-install-project

# App source (expects `generated/` to already exist — see NOTE above).
COPY . .
RUN uv sync --no-dev

# ── Runtime: slim, non-root ───────────────────────────────────────────────────
FROM python:3.12-slim AS runtime

# gRPC health probe port. Keep in sync with GRPC_PORT.
ENV GRPC_PORT=50052 \
    PATH="/opt/venv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

# Dedicated unprivileged user.
RUN groupadd --gid 1001 app \
    && useradd --uid 1001 --gid 1001 --no-create-home --shell /usr/sbin/nologin app

WORKDIR /app

COPY --from=builder /opt/venv /opt/venv
COPY --from=builder /app/src /app/src
COPY --from=builder /app/generated /app/generated

USER 1001

EXPOSE ${GRPC_PORT}

# main.py adds ./generated to sys.path and starts grpc.aio.
CMD ["python", "src/main.py"]
