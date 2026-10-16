"""Flash-sale seeding and meters for the advertised-price scenarios (change order-advertised-price).

Reuses `flash_hold_support` (read-only there) for the campaign seed, the stock meter and the
promotion_db psql access; this module adds what a purchase flow needs on top:

  * `create_campaign`   a campaign on a listing, seeded as the listing-owning seller (an admin
                        CreateCampaign is audited first and fails closed without team-audit)
  * `end_campaign`      the sale ends: its ends_at moves into the past in promotion_db. The Flipt
                        kill-switch is shared state across scenarios, so it is not flipped here.
  * `flash_remaining`   GetFlashSaleStock.remaining (cap - sold - live holds)
  * `flash_sold`        the campaign's stock_sold, read through GetActiveFlashSale at the gateway
                        while the campaign is active

Names come from the environment exactly as in `flash_hold_support` / `plp_stack`
(STACK_NETWORK, PLP_POSTGRES_CONTAINER).
"""

from __future__ import annotations

from tests.e2e.support import flash_hold_support as fh


def create_campaign(
    seller_token: str,
    listing_id: str,
    sale_price: int,
    stock_cap: int,
    variant_id: str = "",
) -> str:
    """CreateCampaign as the owning seller; returns the campaign id."""
    assert not variant_id, "variant campaigns are not used by the end-to-end scenarios"
    return fh.seed_flash_campaign(seller_token, listing_id, stock_cap, sale_price)


def end_campaign(campaign_id: str) -> None:
    """The sale is over: ends_at is one minute in the past, so it is no longer active."""
    fh.psql(
        "UPDATE flash_sale_campaigns SET ends_at = now() - interval '1 minute' "
        f"WHERE id = {fh.sql_lit(campaign_id)}"
    )


def flash_remaining(campaign_id: str) -> int:
    return fh.stock(campaign_id)["remaining"]


def flash_sold(campaign_id: str) -> int:
    """stock_sold straight from the campaign row (committed units; a hold counts only once committed)."""
    out = fh.psql(
        f"SELECT stock_sold FROM flash_sale_campaigns WHERE id = {fh.sql_lit(campaign_id)}"
    ).stdout.strip()
    assert out, f"campaign {campaign_id} not found in promotion_db"
    return int(out)
