# MLflow tracking/registry server, pinned. The official image ships neither the
# Postgres driver nor boto3, both needed (backend store + MinIO artifacts).
FROM ghcr.io/mlflow/mlflow:v3.2.0
RUN pip install --no-cache-dir psycopg2-binary boto3
