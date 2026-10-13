"""Scope table for the AIService RPCs (who may call what).

One module holds the RPC -> required-scope mapping so a descriptor test fails when a
new ``AIService`` RPC ships without a gate. The service enforces scopes on the
principal the gateway forwards; it does not authenticate.

``ShoppingAssistant`` is absent from the table on purpose: ``ai:use`` is not granted
to any role yet, so it is enforced only when ``AI_USE_SCOPE_REQUIRED=true`` (see
``ai_use_scopes``).
"""

from typing import Final

AI_USE_SCOPE: Final = "ai:use"
LISTING_WRITE_SCOPE: Final = "listing.write"
LISTING_READ_SCOPE: Final = "listing.read"
ADMIN_SCOPE: Final = "admin"
# Internal only: held by service principals, never granted to a user role.
AI_CLASSIFY_SCOPE: Final = "ai.classify"

# platform.ai.v1.AIService: RPC name -> scopes the principal must hold (all).
AI_SERVICE_SCOPES: Final[dict[str, tuple[str, ...]]] = {
    "MagicListing": (LISTING_WRITE_SCOPE,),
    "ChatCopilot": (LISTING_WRITE_SCOPE,),
    "SummarizeReviews": (LISTING_READ_SCOPE,),
    "ClassifyTags": (AI_CLASSIFY_SCOPE,),
}


def ai_use_scopes(required: bool) -> tuple[str, ...]:
    """Scopes for the shopper LLM RPCs: empty until ``AI_USE_SCOPE_REQUIRED``, then ``ai:use``."""
    return (AI_USE_SCOPE,) if required else ()
