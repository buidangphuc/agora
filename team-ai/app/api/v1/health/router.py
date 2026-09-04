from fastapi import APIRouter, Request, status
from fastapi.responses import JSONResponse, PlainTextResponse

from app.bootstrap.state import get_health_service
from app.core import grpc_metrics

router = APIRouter(tags=["health"])


async def _metrics() -> PlainTextResponse:
    # Prometheus scrape target — gRPC request counter for KEDA autoscaling.
    return PlainTextResponse(
        grpc_metrics.render(), media_type="text/plain; version=0.0.4"
    )


async def _liveness() -> dict[str, str]:
    return {"status": "ok"}


async def _readiness(request: Request) -> JSONResponse | dict[str, object]:
    result = await get_health_service(request.app).readiness()
    payload = {
        "status": result.status,
        "dependencies": result.dependencies,
    }
    if result.status != "ok":
        return JSONResponse(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE, content=payload
        )
    return payload


router.add_api_route("/healthz", _liveness, methods=["GET"], name="liveness")
router.add_api_route(
    "/readyz", _readiness, methods=["GET"], name="readiness", response_model=None
)
router.add_api_route("/metrics", _metrics, methods=["GET"], name="metrics")
