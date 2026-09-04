# Spark + Python image running the offline ALS batch job. Bitnami Spark ships a
# JVM + a matching PySpark, so `python -m recsys` runs in local[*] with no cluster.
# The container's entrypoint IS the job (a CronJob invokes it); there is no server.
FROM bitnami/spark:3.5.1

USER root
WORKDIR /app

# Runtime Python deps. pyspark is already provided by the image's Spark install,
# so it is dropped from the container requirements to avoid a version clash.
COPY requirements.txt ./
RUN grep -v '^pyspark' requirements.txt > /tmp/reqs.txt \
    && pip install --no-cache-dir -r /tmp/reqs.txt

COPY recsys/ ./recsys/
COPY sample_data/ ./sample_data/

# Non-root for runtime (Bitnami convention: uid 1001).
USER 1001

# Batch entrypoint. Config is entirely env-driven (see .env.example); the
# platform-gitops CronJob supplies WAREHOUSE_DRIVER, QDRANT_URL, REDIS_*, ALS knobs.
ENTRYPOINT ["python", "-m", "recsys"]
