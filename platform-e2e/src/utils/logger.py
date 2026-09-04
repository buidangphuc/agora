"""Lightweight scenario logger.

Mirrors bds `CucumberLogger`: logs go to stdout (captured by pytest) and are also
buffered so hooks can attach them to the HTML report on failure.
"""

from __future__ import annotations

import logging

_LOG_FORMAT = "%(asctime)s %(levelname)s %(name)s :: %(message)s"
logging.basicConfig(level=logging.INFO, format=_LOG_FORMAT)


class ScenarioLogger:
    def __init__(self, scenario_name: str) -> None:
        self._log = logging.getLogger("e2e")
        self._scenario = scenario_name
        self.buffer: list[str] = []

    def _emit(self, level: int, msg: str) -> None:
        line = f"[{self._scenario}] {msg}"
        self.buffer.append(line)
        self._log.log(level, line)

    def info(self, msg: str) -> None:
        self._emit(logging.INFO, msg)

    def warning(self, msg: str) -> None:
        self._emit(logging.WARNING, msg)

    def error(self, msg: str) -> None:
        self._emit(logging.ERROR, msg)
