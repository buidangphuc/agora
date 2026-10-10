"""Scope table for the AIService / ChatService RPCs (who may call what).

One module holds the RPC -> required-scope mapping so a descriptor test can fail
when a new ``AIService`` RPC ships without a gate. team-ai enforces scopes on the
principal the gateway forwards (ADR-0003/0006); it does not authenticate.

Scopes team-identity grants today (``internal/authz/scopes.go``): buyer
``listing.read``; seller adds ``listing.write``; admin adds ``admin``. The seller
gates (``listing.write``) and the public read gate (``listing.read``) are
therefore enforced unconditionally.

``ai:use`` (ShoppingAssistant / StreamChat) is NOT granted to any role yet, and
the public ``/assistant`` page is reachable logged-out, so requiring it today
would deny every buyer. It is enforced only when ``AI_USE_SCOPE_REQUIRED=true``;
flip that on once team-identity grants ``ai:use`` to buyer, seller and admin
(and ``PUBLIC_SCOPES`` in team-gateway stays free of it). Tokens minted before the
identity change carry no ``ai:use`` until they expire (one JWT TTL).

Anonymous policy once enforced: deny on every LLM-capable RPC. The review summary
stays public because it is stateless and rendered on the product page.
"""

from __future__ import annotations

from typing import Final

AI_USE_SCOPE: Final = "ai:use"
LISTING_WRITE_SCOPE: Final = "listing.write"
LISTING_READ_SCOPE: Final = "listing.read"
ADMIN_SCOPE: Final = "admin"
# Internal only: held by service principals (team-search's indexer), never granted to a
# user role by team-identity and never in the gateway's public scopes.
AI_CLASSIFY_SCOPE: Final = "ai.classify"

# platform.ai.v1.AIService: RPC name -> scopes the principal must hold (all).
# ShoppingAssistant is absent on purpose: see ``ai_use_scopes``.
AI_SERVICE_SCOPES: Final[dict[str, tuple[str, ...]]] = {
    # Seller features: seller and admin hold listing.write, buyer/anonymous do not.
    "MagicListing": (LISTING_WRITE_SCOPE,),
    "ChatCopilot": (LISTING_WRITE_SCOPE,),  # + seller_id rule, see AIServicer
    # Stateless, rendered on the public product page: anonymous keeps it.
    "SummarizeReviews": (LISTING_READ_SCOPE,),
    # Internal service-to-service (see AIServicer.ClassifyTags: also requires a service principal).
    "ClassifyTags": (AI_CLASSIFY_SCOPE,),
}


def ai_use_scopes(required: bool) -> tuple[str, ...]:
    """Scopes for the shopper LLM RPCs (ShoppingAssistant, StreamChat).

    Empty (open, as before) until ``AI_USE_SCOPE_REQUIRED`` is on; then ``ai:use``.
    """
    return (AI_USE_SCOPE,) if required else ()
