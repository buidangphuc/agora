"""In-process gRPC request counter, exposed at HTTP ``/metrics`` for Prometheus.

The service is IO-bound, so request rate (not CPU) is the autoscaling signal: KEDA
scales on ``rate(teamai_grpc_requests_total[..])``. asyncio is single-threaded, so a
plain counter is safe.
"""

_total = 0


def hit() -> None:
    """Count one handled gRPC request. Called once per RPC by the tracing interceptor."""
    global _total
    _total += 1


def render() -> str:
    return (
        "# HELP teamai_grpc_requests_total Total gRPC requests handled.\n"
        "# TYPE teamai_grpc_requests_total counter\n"
        f"teamai_grpc_requests_total {_total}\n"
    )


def reset() -> None:
    """Test helper: zero the counter."""
    global _total
    _total = 0
