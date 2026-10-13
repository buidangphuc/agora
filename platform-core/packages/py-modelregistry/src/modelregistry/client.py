"""MLflow client with a per-call timeout cap, a shared operation deadline and error translation."""
from __future__ import annotations

import os
import re
import threading
import time
from collections.abc import Callable
from typing import Any, TypeVar

import requests
from mlflow import MlflowClient
from mlflow.exceptions import MlflowException

from .errors import RegistryUnavailable

T = TypeVar("T")

CALL_TIMEOUT_S = 10
OPERATION_DEADLINE_S = 12.0
_TIMEOUT_ENV = "MLFLOW_HTTP_REQUEST_TIMEOUT"
_RETRIES_ENV = "MLFLOW_HTTP_REQUEST_MAX_RETRIES"
_CONN_RE = re.compile(
    r"max retries exceeded|connection|timed out|timeout|failed with exception|"
    r"too many (5\d\d|error)|temporarily unavailable|name resolution",
    re.IGNORECASE,
)


def _cap_timeout() -> None:
    """Larger or invalid MLFLOW_HTTP_REQUEST_TIMEOUT values are ignored; smaller ones are kept."""
    raw = os.environ.get(_TIMEOUT_ENV)
    try:
        value = float(raw) if raw is not None else None
    except ValueError:
        value = None
    if value is None or value <= 0 or value > CALL_TIMEOUT_S:
        os.environ[_TIMEOUT_ENV] = str(CALL_TIMEOUT_S)
    # No MLflow-internal retries: nothing may be retried after the operation deadline.
    try:
        retries = int(os.environ.get(_RETRIES_ENV, ""))
    except ValueError:
        retries = None
    if retries is None or retries > 0:
        os.environ[_RETRIES_ENV] = "0"


def _translate(exc: Exception) -> Exception:
    if isinstance(exc, RegistryUnavailable):
        return exc
    if isinstance(exc, (requests.ConnectionError, requests.Timeout, ConnectionError, TimeoutError)):
        return RegistryUnavailable(f"registry unavailable: {exc}")
    if (
        isinstance(exc, MlflowException)
        and _CONN_RE.search(str(exc))
        and getattr(exc, "error_code", "")
        not in ("RESOURCE_DOES_NOT_EXIST", "RESOURCE_ALREADY_EXISTS", "INVALID_PARAMETER_VALUE")
    ):
        return RegistryUnavailable(f"registry unavailable: {exc}")
    return exc


class Session:
    """One library operation: a client (env read now) and a 12 s monotonic deadline."""

    def __init__(self) -> None:
        _cap_timeout()
        self.client = MlflowClient()
        self._end = time.monotonic() + OPERATION_DEADLINE_S

    def call(self, fn: Callable[..., T], *args: Any, **kwargs: Any) -> T:
        remaining = self._end - time.monotonic()
        if remaining <= 0:
            raise RegistryUnavailable("registry operation deadline (12 s) exceeded")
        box: dict[str, Any] = {}

        def run() -> None:
            try:
                box["value"] = fn(*args, **kwargs)
            except Exception as e:  # noqa: BLE001 - re-raised in the caller
                box["error"] = e

        worker = threading.Thread(target=run, daemon=True)
        worker.start()
        worker.join(remaining)
        if worker.is_alive():
            raise RegistryUnavailable("registry operation deadline (12 s) exceeded")
        if "error" in box:
            err = _translate(box["error"])
            if err is box["error"]:
                raise err
            raise err from box["error"]
        return box["value"]  # type: ignore[no-any-return]


def check_reachable() -> None:
    """Pre-flight for trainers: raise RegistryUnavailable unless the registry answers."""
    s = Session()
    s.call(s.client.search_registered_models, max_results=1)
