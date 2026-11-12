"""Black-box access to the service-only SearchService.GetListingStock (change search-listing-stock-lookup).

The RPC is not routed by the gateway and team-search publishes no host port, so it is driven with
`grpcurl` (functional calls) and `ghz` (latency) from throwaway containers on the stack network,
with the `x-principal-*` headers the interceptor reads.

    STACK_NETWORK              docker network of the stack (default agora_default)
    LSL_SEARCH_ADDR            default team-search-svc:50052
    LSL_GRPCURL_IMAGE          default fullstorydev/grpcurl:latest
    LSL_GHZ_IMAGE              default ghcr.io/bojand/ghz:0.120.0
    LSL_PROTO_DIR              proto root holding platform/search/v1/search.proto
                               (default team-search/proto of this checkout)
    LSL_OPENSEARCH_CONTAINER   default: REDIS_CONTAINER with its redis-1 suffix replaced by opensearch-1
    SEARCH_CONTAINER           team-search container (settings.search_container)
"""

from __future__ import annotations

import json
import os
import re
import time
from datetime import datetime
from pathlib import Path
from typing import Any

from config.settings import get_settings
from tests.e2e.support import plp_stack as stack

PROTO_FILE = "platform/search/v1/search.proto"
SERVICE = "platform.search.v1.SearchService"
METHOD = "GetListingStock"
SCOPE = "search:read"

PRINCIPALS: dict[str, tuple[str, str, str]] = {
    "service": ("service-team-ai", "service", SCOPE),
    "user": ("lsl-user", "user", SCOPE),
    "service-no-scope": ("service-no-scope", "service", ""),
}


def search_addr() -> str:
    return os.getenv("LSL_SEARCH_ADDR", "team-search-svc:50052")


def grpcurl_image() -> str:
    return os.getenv("LSL_GRPCURL_IMAGE", "fullstorydev/grpcurl:latest")


def ghz_image() -> str:
    return os.getenv("LSL_GHZ_IMAGE", "ghcr.io/bojand/ghz:0.120.0")


def opensearch_container() -> str:
    explicit = os.getenv("LSL_OPENSEARCH_CONTAINER")
    if explicit:
        return explicit
    redis = os.getenv("REDIS_CONTAINER", "agora-redis-1")
    return re.sub(r"redis-1$", "opensearch-1", redis)


def search_container() -> str:
    return get_settings().search_container


def proto_dir() -> Path:
    env = os.getenv("LSL_PROTO_DIR")
    if env:
        return Path(env)
    return Path(__file__).resolve().parents[4] / "team-search" / "proto"


def _snake(grpc_code: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "_", grpc_code).lower()


def _headers(principal: str | None) -> dict[str, str]:
    if principal is None:
        return {}
    pid, ptype, scopes = PRINCIPALS[principal]
    headers = {"x-principal-id": pid, "x-principal-type": ptype}
    if scopes:
        headers["x-principal-scopes"] = scopes
    return headers


def _require_proto() -> Path:
    pdir = proto_dir()
    assert (pdir / PROTO_FILE).is_file(), f"search proto not found under {pdir} (LSL_PROTO_DIR)"
    return pdir


def call(ids: list[str], principal: str | None = "service") -> tuple[str, Any]:
    """One GetListingStock call: (connect-style code, response dict or error text); `ok` on success."""
    pdir = _require_proto()
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        grpcurl_image(), "-plaintext", "-import-path", "/proto", "-proto", PROTO_FILE,
    ]  # fmt: skip
    for key, value in _headers(principal).items():
        cmd += ["-H", f"{key}: {value}"]
    cmd += ["-d", json.dumps({"listingIds": ids}), search_addr(), f"{SERVICE}/{METHOD}"]
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", json.loads(proc.stdout or "{}")
    err = (proc.stderr + proc.stdout).strip()
    m = re.search(r"Code:\s*(\w+)", err)
    return (_snake(m.group(1)) if m else f"grpcurl_exit_{proc.returncode}"), err


def entries(body: Any) -> list[dict[str, Any]]:
    assert isinstance(body, dict), f"expected a response body, got {body!r}"
    return body.get("listings", [])


def stock_by_entry(body: Any) -> list[tuple[str, int | None]]:
    """(listing_id, stock) per entry in order; stock is None when absent, 0 when present as 0."""
    return [
        (e["listingId"], int(e["stock"]) if "stock" in e else None) for e in entries(body)
    ]


def ghz(ids: list[str], calls: int, principal: str = "service") -> dict[str, Any]:
    """`calls` sequential GetListingStock calls on one connection; ghz's JSON report."""
    pdir = _require_proto()
    meta = json.dumps(_headers(principal))
    proc = stack.docker(
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        ghz_image(), "--insecure", "--proto", f"/proto/{PROTO_FILE}", "-i", "/proto",
        "--call", f"{SERVICE}.{METHOD}", "-d", json.dumps({"listingIds": ids}), "-m", meta,
        "-n", str(calls), "-c", "1", "--format", "json", search_addr(),
        check=False, timeout=300,
    )  # fmt: skip
    assert proc.returncode == 0, f"ghz failed: {(proc.stderr + proc.stdout).strip()}"
    return json.loads(proc.stdout)


def percentile_ms(report: dict[str, Any], pct: int) -> float:
    for item in report["latencyDistribution"] or []:
        if item["percentage"] == pct:
            return item["latency"] / 1e6
    raise AssertionError(f"ghz report has no p{pct}: {report.get('latencyDistribution')}")


def status_codes(report: dict[str, Any]) -> dict[str, int]:
    return report.get("statusCodeDistribution") or {}


def logs_since(since_unix: float) -> str:
    proc = stack.docker("logs", "--since", str(int(since_unix)), search_container(), check=False)
    return proc.stdout + proc.stderr


def _line_time(line: str) -> float | None:
    try:
        stamp = json.loads(line)["time"]
    except (ValueError, KeyError, TypeError):
        return None
    m = re.match(r"(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(\.\d+)?(Z|[+-]\d\d:\d\d)$", str(stamp))
    if not m:
        return None
    frac = (m.group(2) or ".0")[:7]
    zone = "+00:00" if m.group(3) == "Z" else m.group(3)
    return datetime.fromisoformat(f"{m.group(1)}{frac}{zone}").timestamp()


def lines_after(container: str, started: float) -> list[str]:
    proc = stack.docker("logs", "--since", str(int(started) - 1), container, check=False)
    kept = []
    for line in (proc.stdout + proc.stderr).splitlines():
        at = _line_time(line)
        if at is not None and at >= started:
            kept.append(line)
    return kept


def gateway_edge_line(started: float, request_id: str) -> dict[str, Any] | None:
    for raw in lines_after(get_settings().gateway_container, started):
        if request_id not in raw or '"edge.request"' not in raw:
            continue
        try:
            line = json.loads(raw)
        except ValueError:
            continue
        if line.get("msg") == "edge.request" and line.get("request_id") == request_id:
            return line
    return None


def search_calls_after(started: float) -> list[str]:
    return [ln for ln in lines_after(search_container(), started) if METHOD in ln]


def pause_opensearch() -> None:
    stack.docker("pause", opensearch_container())


def unpause_opensearch() -> None:
    stack.docker("unpause", opensearch_container(), check=False)


def stop_opensearch() -> None:
    stack.docker("stop", opensearch_container())


def start_opensearch() -> None:
    stack.docker("start", opensearch_container(), check=False)


def wait_search_healthy(timeout_s: float = 90.0) -> None:
    from tests.e2e.flows.stack_flow import wait_for_search

    wait_for_search(get_settings().gateway_url, timeout_s)


def now() -> float:
    return time.time()
