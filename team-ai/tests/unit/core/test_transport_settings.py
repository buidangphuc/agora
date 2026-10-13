import pytest
from pydantic import ValidationError

from app.core.config import Settings
from tests.factories import build_test_settings


def test_transport_defaults_match_team_ai_contract():
    settings = build_test_settings()

    assert settings.GRPC_HOST == "0.0.0.0"
    assert Settings.model_fields["GRPC_PORT"].default == 50051
    assert settings.GRPC_GRACE_SECONDS == 10.0
    assert settings.AI_USE_SCOPE_REQUIRED is False


@pytest.mark.parametrize(
    ("field", "value"),
    [("GRPC_PORT", -1), ("GRPC_GRACE_SECONDS", -1)],
)
def test_transport_numeric_bounds(field, value):
    with pytest.raises(ValidationError):
        build_test_settings(**{field: value})


def test_grpc_port_zero_means_an_ephemeral_port():
    assert build_test_settings(GRPC_PORT=0).GRPC_PORT == 0
