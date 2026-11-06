FROM python:3.12-slim AS base
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
RUN useradd --system --uid 1001 --no-create-home app \
    && mkdir -p /features && chown 1001:1001 /features
USER 1001

# docker build --target test .
FROM base AS test
CMD ["pytest", "-v", "tests/"]

# default (last) stage: the batch job
FROM base AS job
ENV FEATURESTORE_INPUT_DIR=/data FEATURESTORE_OFFLINE_DIR=/features
ENTRYPOINT ["python", "-m", "featurestore"]
CMD ["materialize"]
