"""Seed a published listing via the gateway API (precondition setup).

Used by the `needsListing` tag hook and by scenarios that need a known product to
exist without going through the seller UI.
"""

from __future__ import annotations

from src.api.services import AuthService, ListingService
from src.models import Listing, User


def seed_seller_account(world, username: str, password: str) -> User:
    """Register (idempotent) a seller and capture its token on the service factory."""
    auth: AuthService = world.service_factory.auth
    token = auth.register(username, password, role="seller")
    world.service_factory.set_token(token)
    seller = User(username=username, password=password, role="seller", token=token)
    world.state.seeded_seller = seller
    return seller


def seed_listing(world, listing: Listing, seller: User) -> Listing:
    """Create a listing as the given seller. Requires the seller token to be active."""
    if world.service_factory._token != seller.token:  # noqa: SLF001 - internal sync
        world.service_factory.set_token(seller.token)
    svc: ListingService = world.service_factory.listing
    listing_id = svc.create_listing(listing)
    listing.listing_id = listing_id
    world.state.listing = listing
    world.logger.info(f"Seeded listing {listing_id} '{listing.title}'")
    return listing
