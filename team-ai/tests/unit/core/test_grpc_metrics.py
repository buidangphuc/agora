from app.core import grpc_metrics


def test_render_is_prometheus_text_with_the_counter() -> None:
    grpc_metrics.reset()
    for _ in range(3):
        grpc_metrics.hit()
    text = grpc_metrics.render()
    assert "# TYPE teamai_grpc_requests_total counter" in text
    assert "teamai_grpc_requests_total 3\n" in text


def test_reset_zeroes_the_counter() -> None:
    grpc_metrics.hit()
    grpc_metrics.reset()
    assert "teamai_grpc_requests_total 0\n" in grpc_metrics.render()
