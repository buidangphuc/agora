import importlib.util

from app.core.config import Settings
from app.modules.business.listing_screen.mock import MockScreenService
from app.modules.business.listing_screen.ports import ScreenService


def build_screen_service(settings: Settings) -> ScreenService:
    match settings.SCREEN_BACKEND:
        case "mock":
            return MockScreenService(settings.SCREEN_MOCK_MARKER)
        case "llm_router":
            # Fail at boot: the router path needs the optional `ai` extra.
            if importlib.util.find_spec("langchain_core") is None:
                raise RuntimeError(
                    "SCREEN_BACKEND='llm_router' needs the 'ai' extra "
                    "(langchain, langchain-openai); install with --extra ai."
                )
            return _build_llm_screen_service(settings)


def _build_llm_screen_service(settings: Settings) -> ScreenService:
    from app.modules.ai.llm.runtime import build_llm_instance
    from app.modules.business.listing_screen.service import LLMScreenService

    llm = build_llm_instance(
        settings,
        instance_id="grpc-screen",
        service_name="team-ai.screen",
        tags=("grpc", "screen"),
    )
    return LLMScreenService(llm, timeout_seconds=settings.SCREEN_TIMEOUT_SECONDS)
