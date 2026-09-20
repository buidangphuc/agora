"""Uvicorn entrypoint for modelserve router."""

from __future__ import annotations

import uvicorn

from modelserve.config import settings
from modelserve.router import app


def run() -> None:
    uvicorn.run(
        app,
        host=settings.router_host,
        port=settings.router_port,
        log_level="info",
    )


if __name__ == "__main__":
    run()
