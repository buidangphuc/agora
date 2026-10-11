# Python + JRE image running the offline ALS batch job. PySpark (pinned in
# requirements.txt) bundles Spark itself, so `python -m recsys` runs in local[*]
# with no cluster; it only needs a Java runtime on PATH. (The former bitnami/spark
# base is no longer published on Docker Hub.) Spark 3.5 supports Java 17.
# The container's entrypoint IS the job (a CronJob invokes it); there is no server.
FROM python:3.11-slim-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends openjdk-17-jre-headless procps \
    && rm -rf /var/lib/apt/lists/*
ENV PYTHONUNBUFFERED=1

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY recsys/ ./recsys/
COPY sample_data/ ./sample_data/

# Non-root for runtime; Spark needs a writable HOME for its scratch/ivy dirs.
RUN useradd --uid 1001 --create-home recsys
USER 1001

# Batch entrypoint. Config is entirely env-driven (see .env.example); the
# platform-gitops CronJob supplies DATASET_DIR, QDRANT_URL, REDIS_*, ALS knobs.
ENTRYPOINT ["python", "-m", "recsys"]
