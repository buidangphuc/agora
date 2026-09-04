"""pytest + pytest-bdd wiring (mirrors bds hooks.ts + browser-manager + world).

- Overrides pytest-playwright launch/context args from Settings (headless, device).
- Provides the `world` fixture (per-scenario World, function-scoped) — the
  equivalent of Cucumber's per-scenario World; `browser` stays session-scoped
  (via pytest-playwright) while context/page are per-scenario so state never leaks.
- `seed_by_tags` autouse fixture reads a scenario's markers and seeds accounts /
  listings before steps run (equivalent of bds tag-scoped Before hooks).
- `pytest_bdd_step_error` attaches a screenshot on failure.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from config.browsers import launch_options
from config.devices import is_mobile_device
from config.settings import get_settings
from config.tags import ROLE_SEED_MARKERS, TAG_PAYLOAD_MAP
from src.models import Listing, User
from src.utils import data as fake
from tests.e2e.flows import seed_listing
from tests.e2e.support.world import World

SETTINGS = get_settings()
SCREENSHOT_DIR = Path(__file__).resolve().parent.parent / "test-results" / "screenshots"


# ── pytest-playwright overrides ──────────────────────────────────────────
@pytest.fixture(scope="session")
def is_headless() -> bool:
    return bool(SETTINGS.headless)


@pytest.fixture(scope="session")
def browser_type_launch_args(browser_type_launch_args):  # noqa: ANN001
    return {**browser_type_launch_args, **launch_options(headless=SETTINGS.headless)}


@pytest.fixture
def browser_context_args(browser_context_args, playwright):  # noqa: ANN001
    args = {**browser_context_args, "ignore_https_errors": True}
    device = SETTINGS.device
    if is_mobile_device(device):
        args = {**args, **playwright.devices[device]}
    else:
        args.setdefault("viewport", {"width": 1280, "height": 720})
    return args


# ── World ────────────────────────────────────────────────────────────────
@pytest.fixture
def world(context, page, request):  # noqa: ANN001
    page.set_default_timeout(SETTINGS.action_timeout_ms)
    page.set_default_navigation_timeout(SETTINGS.navigation_timeout_ms)
    w = World(context, page, request.node.name)
    w.setup_web_context()
    yield w
    w.cleanup()


# ── Tag-scoped seeding (bds TAG_PAYLOAD_MAP equivalent) ──────────────────
def _seed_account(world: World, role: str) -> User:
    username = fake.unique_username(role)
    token = world.service_factory.auth.register(username, SETTINGS.seed_password, role)
    world.service_factory.set_token(token)
    user = User(username=username, password=SETTINGS.seed_password, role=role, token=token)
    if role == "seller":
        world.state.seeded_seller = user
    world.state.extra[f"seeded_{role}"] = user
    world.logger.info(f"Seeded {role} account {username}")
    return user


@pytest.fixture(autouse=True)
def seed_by_tags(request, world):  # noqa: ANN001
    markers = {m.name for m in request.node.iter_markers()}

    for marker, role in ROLE_SEED_MARKERS.items():
        if marker in markers:
            _seed_account(world, role)

    for marker, seed in TAG_PAYLOAD_MAP.items():
        if marker in markers:
            seller = world.state.seeded_seller or _seed_account(world, "seller")
            listing = Listing(
                title=f"{seed.title} {fake.price_vnd():d}",
                category_id=seed.category_id,
                price=seed.price,
                stock=seed.stock,
                status=seed.status,
                description=seed.description,
            )
            seed_listing(world, listing, seller)

    if "needsAddress" in markers or "needsOrder" in markers:
        buyer = world.state.extra.get("seeded_buyer") or _seed_account(world, "buyer")
        world.service_factory.set_token(buyer.token)
        addr_res = world.service_factory.address.create_address(
            recipient_name="Nguyen Van A",
            phone="0912345678",
            street="29 Lieu Giai",
            city="Ha Noi",
            ward="Phuong Lieu Giai",
            district="Quan Ba Dinh",
            is_default=True,
        )
        addr_id = addr_res.get("address", {}).get("id", "")
        world.state.extra["address_id"] = addr_id

    if "needsOrder" in markers:
        buyer = world.state.extra.get("seeded_buyer") or _seed_account(world, "buyer")
        seller = world.state.seeded_seller or _seed_account(world, "seller")
        if not world.state.listing:
            listing = Listing(
                title=f"[E2E] Seeded Listing {fake.price_vnd():d}",
                category_id="cat-electronics",
                price=5_000_000,
                stock=100,
                status="published",
                description="Sản phẩm seed tự động cho Order E2E.",
            )
            seed_listing(world, listing, seller)
        world.service_factory.set_token(buyer.token)
        world.service_factory.cart.add_to_cart(
            listing_id=world.state.listing.listing_id, quantity=1
        )
        order_res = world.service_factory.order.create_order(
            {"paymentMethod": "PAYMENT_METHOD_COD"}
        )
        orders = order_res.get("orders", [])
        order_id = orders[0].get("id") if orders else order_res.get("order", {}).get("id", "")
        world.state.order_id = order_id
        world.state.extra["order_id"] = order_id
    yield


# ── Failure screenshot (bds AfterStep) ───────────────────────────────────
def pytest_bdd_step_error(
    request, feature, scenario, step, step_func, step_func_args, exception
):  # noqa: ANN001, PLR0913
    if world is None:
        return
    try:
        SCREENSHOT_DIR.mkdir(parents=True, exist_ok=True)
        safe = scenario.name.replace(" ", "_").replace("/", "_")[:60]
        path = SCREENSHOT_DIR / f"{safe}.png"
        world.page.screenshot(path=str(path), full_page=True)
        world.logger.error(f"Step failed: '{step.name}' -> screenshot {path}")
    except Exception:  # noqa: BLE001
        pass
