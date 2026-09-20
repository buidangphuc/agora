## Why

ML repositories (`platform-recsys`, `team-ai`, `platform-modelserve`) lack automated CI pipelines. Code changes and drift can enter without verification of:
1. Syntax and linting standards (`ruff`, `black`, `mypy`).
2. Environment drift gates (matching `.env.example` against config definitions).
3. ML offline evaluation metrics regressions.
4. Contract conformance between services (`team-ai` $\leftrightarrow$ `platform-modelserve`).

Following **P1-T3**, this change introduces GitHub Actions CI workflows for the ML repositories.

## What Changes

- **platform-recsys** (`.github/workflows/ci.yaml`):
  - Checkout, setup Python 3.10+, install dependencies.
  - Run linting (`ruff check`, `black --check`).
  - Run byte-compile syntax check.
  - Run config & environment drift checks (`test_env_drift.py`).
  - Run unit tests and offline evaluation suite (`test_evals.py`).
- **team-ai** (`.github/workflows/ci.yaml`):
  - Checkout, setup Python, install requirements.
  - Run lint & pytest test suite for AI modules, RAG, and embeddings contract.
- **platform-modelserve** (`.github/workflows/ci.yaml`):
  - Checkout, setup Python, install dependencies.
  - Run ruff lint and env drift tests.
  - Run unit tests, admission control tests, Redis cache tests, and contract conformance tests against `_extract_vectors`.

## Non-goals

- No heavy GPU cluster requirements for standard PR CI — offline checks and mocked inference run on standard CPU runners.
