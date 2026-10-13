# Weekly fine-tuning job image (add-platform-finetune). The container's entrypoint IS the job
# (a CronJob / `docker compose --profile jobs run` invokes it); there is no server.
#
# CPU-buildable: nothing here needs a GPU at build time. PyPI's linux torch wheel bundles the CUDA
# runtime, so the same image trains on the nvidia.com/gpu node and builds on a plain CPU host.
#
# Build context is this directory (docker build platform-finetune). py-modelregistry lives in the
# monorepo (platform-core/packages/py-modelregistry), outside that context, so docker-compose.services.yaml
# passes it as the named build context `modelregistry`. Without it (plain `docker build`) the image
# still builds but has no registry library, and `python main.py run` fails closed on import.
FROM scratch AS modelregistry

FROM python:3.11-slim-bookworm
ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    UV_SYSTEM_PYTHON=1 \
    MLFLOW_TRACKING_URI=http://mlflow:5050
# MODELREGISTRY_POLICIES is deliberately unset: the policies.yaml packaged in py-modelregistry holds the
# finetune-classifier gate. Set it only to override with another file.

WORKDIR /app

# uv, not pip: the full `.[dev,cpu]` extras (dvc, sagemaker, triton-model-analyzer, ...) make pip's resolver
# hang, and `dev` also names the monorepo-only `modelregistry`. So the training subset is installed
# explicitly (same set the CI job uses, plus boto3 for the MinIO artifact store). mlflow is pinned to
# the range py-modelregistry's mlflow-skinny requires.
RUN pip install uv
COPY --from=modelregistry / /tmp/modelregistry/
RUN if [ -f /tmp/modelregistry/pyproject.toml ]; then uv pip install /tmp/modelregistry; fi; \
    rm -rf /tmp/modelregistry
RUN uv pip install \
      "mlflow>=3.2,<3.3" boto3 torch "transformers>=5.0" datasets accelerate peft scikit-learn \
      "optimum[onnxruntime]" onnxruntime pandas pyYAML python-box ensure tqdm joblib matplotlib \
      fastapi uvicorn prometheus-client numpy loguru

COPY pyproject.toml README.md main.py ./
COPY src/ ./src/
COPY config/ ./config/
# Editable, no deps (installed above): artifact paths resolve relative to /app.
RUN uv pip install --no-deps -e .

RUN useradd --uid 1001 --create-home finetune && mkdir -p /app/artifacts /app/logs && chown -R 1001 /app/artifacts /app/logs
USER 1001

ENTRYPOINT ["python", "main.py", "run"]
