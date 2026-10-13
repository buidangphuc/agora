import importlib.util
from typing import TYPE_CHECKING, Final

from app.core.config import Settings
from app.modules.business.chat.echo import EchoChatService
from app.modules.business.chat.ports import ChatService

if TYPE_CHECKING:
    from redis.asyncio import Redis

    from app.modules.platform.quota.service import QuotaService

CHAT_QUOTA_RESOURCE: Final = "chat.reply"


def build_chat_service(
    settings: Settings,
    *,
    quota: "QuotaService",
    redis: "Redis",
) -> ChatService:
    match settings.CHAT_BACKEND:
        case "mock":
            return EchoChatService()
        case "llm_router":
            # Fail at boot, not on every chat: the router path needs the
            # optional `ai` extra, which the core image omits.
            if importlib.util.find_spec("langchain_core") is None:
                raise RuntimeError(
                    "CHAT_BACKEND='llm_router' needs the 'ai' extra "
                    "(langchain, langchain-openai); install with --extra ai."
                )
            return _build_llm_chat_service(settings, quota=quota, redis=redis)


def _build_llm_chat_service(
    settings: Settings,
    *,
    quota: "QuotaService",
    redis: "Redis",
) -> ChatService:
    from app.modules.ai.llm.prompt import STATIC_SYSTEM_PROMPT, PromptProvider
    from app.modules.ai.llm.runtime import build_llm_instance
    from app.modules.ai.llm.session import RedisSessionStore
    from app.modules.ai.llm.usage import CHARS_PER_TOKEN
    from app.modules.business.chat.service import LLMChatService
    from app.modules.platform.quota.models import QuotaPolicy

    llm = build_llm_instance(
        settings,
        instance_id="grpc-chat",
        service_name="team-ai.chat",
        tags=("grpc", "chat"),
    )
    ttl = settings.CHAT_HISTORY_TTL_SECONDS
    turns = settings.CHAT_HISTORY_MAX_TURNS
    chars = settings.CHAT_HISTORY_MAX_TOKENS * CHARS_PER_TOKEN
    sessions = RedisSessionStore(
        redis, ttl_seconds=ttl, max_turns=turns, max_chars=chars
    )
    return LLMChatService(
        llm,
        first_token_timeout=settings.CHAT_FIRST_TOKEN_TIMEOUT_SECONDS,
        quota=quota,
        quota_policy=QuotaPolicy(
            resource=CHAT_QUOTA_RESOURCE,
            limit=settings.QUOTA_CHAT_REPLIES_PER_WINDOW,
            window_seconds=settings.QUOTA_CHAT_WINDOW_SECONDS,
        ),
        sessions=sessions,
        prompts=PromptProvider(
            llm.tracker,
            fallback=settings.CHAT_SYSTEM_PROMPT.strip() or STATIC_SYSTEM_PROMPT,
        ),
    )
