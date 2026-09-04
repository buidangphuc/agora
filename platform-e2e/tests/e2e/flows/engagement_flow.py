"""Community engagement & Q&A flows."""

from __future__ import annotations

from typing import Any

from src.models import User


def submit_product_question_via_api(
    world, user: User, listing_id: str, question_text: str
) -> dict[str, Any]:
    """Submit a question on a product via API."""
    world.service_factory.set_token(user.token)
    res = world.service_factory.engagement.ask_question(listing_id, question_text)
    world.logger.info(f"Submitted question '{question_text}' on listing {listing_id}")
    return res


def toggle_favorite_via_api(world, user: User, listing_id: str) -> dict[str, Any]:
    """Toggle product favorite status via API."""
    world.service_factory.set_token(user.token)
    res = world.service_factory.engagement.toggle_favorite(listing_id)
    world.logger.info(f"Toggled favorite on listing {listing_id}")
    return res
