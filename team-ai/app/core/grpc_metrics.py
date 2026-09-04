"""In-process gRPC request counter, exposed at HTTP ``/metrics`` for Prometheus.

team-ai is IO-bound (it waits on model-server / RAG / network), so CPU is a poor
autoscaling signal. Request *rate* reflects real load, so we count gRPC calls here
and let KEDA scale on ``rate(teamai_grpc_requests_total[..])``. No extra dependency —
asyncio is single-threaded so a plain counter is safe.
"""

from __future__ import annotations

_total = 0


def hit() -> None:
    """Count one handled gRPC request. Called once per RPC by the interceptor."""
    global _total
    _total += 1


def render() -> str:
    return (
        "# HELP teamai_grpc_requests_total Total gRPC requests handled.\n"
        "# TYPE teamai_grpc_requests_total counter\n"
        f"teamai_grpc_requests_total {_total}\n"
    )
