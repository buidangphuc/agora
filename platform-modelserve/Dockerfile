FROM python:3.12-alpine

WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY pyproject.toml .
COPY modelserve/ modelserve/

RUN pip install --no-cache-dir -e .

RUN adduser -D -H -u 1001 app
USER 1001

EXPOSE 8100

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD python -c "import urllib.request as u; u.urlopen('http://127.0.0.1:8100/healthz', timeout=2)"

CMD ["python", "-m", "modelserve"]
