## Why

Recommendation and ranking models require consistent user and item features across both offline training (batch warehouse) and online real-time inference (sub-5ms lookup). Training-serving skew occurs when online features are computed differently or have missing values compared to offline training tables.

Following **P3-T2**, this change introduces `platform-featurestore`: an internal platform capability providing unified feature definitions, online Redis feature serving, offline Parquet snapshots, and parity tests.

## What Changes

- **platform-featurestore** (new repo/capability):
  - `featurestore/definitions.py`: Standard schema for user features and item features.
  - `featurestore/online.py`: Low-latency async/sync Redis online feature store with batch `get_online_features` and TTLs.
  - `featurestore/offline.py`: Parquet/DuckDB offline feature reader for model training.
  - `featurestore/parity.py`: Parity validation ensuring zero training-serving skew.
  - Full test suite, `Dockerfile`, `Makefile`, `pyproject.toml`, `FEATURES.yaml`, `README.md`.
- **platform-core** & **AGENTS.md**:
  - Update polyrepo documentation.

## Non-goals

- No external heavyweight SaaS (e.g. Tecton / SageMaker Feature Store).
