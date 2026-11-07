"""Steps for frontend/seo.feature (change: frontend-seo, initiative discovery-seo).

Black-box: pages are fetched over HTTP as a crawler and the raw HTML is parsed for the tags a
crawler reads. Listings, shops, categories and moderation state are seeded through the gateway.
Only the favicon-independent brand scenario (SEO-27) drives a browser.
"""

from __future__ import annotations

import math
import os
import re
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from html.parser import HTMLParser
from urllib.parse import urljoin

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, ListingService
from src.constants import timeouts
from src.models import Listing
from src.utils import data as fake
from tests.e2e.support import audit_support as aud
from tests.e2e.support import user_enforcement_support as ues
from tests.e2e.support.world import World

SETTINGS = get_settings()
SITE = SETTINGS.base_url.rstrip("/")
APPS = {
    "storefront": (SITE, "session"),
    "seller app": (SETTINGS.seller_base_url.rstrip("/"), "seller_session"),
    "admin app": (SETTINGS.admin_base_url.rstrip("/"), "admin_session"),
}
CRAWLER_UA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
LISTING_RPC = "/platform.listing.v1.ListingService/"
PRIVATE_PREFIXES = (
    "/account",
    "/cart",
    "/checkout",
    "/seller",
    "/admin",
    "/login",
    "/register",
    "/chat",
    "/notifications",
    "/favorites",
    "/dev",
    "/api/",
)
DEMO_BRANDS = ("Marketplace Showcase", "Polyrepo", "BATDONGSAN SINH VIÊN")
VIETNAMESE = re.compile(
    "[àáạảãâầấậẩẫăằắặẳẵèéẹẻẽêềếệểễìíịỉĩòóọỏõôồốộổỗơờớợởỡùúụủũưừứựửữỳýỵỷỹđ]", re.I
)
SITEMAP_WAIT_S = 90
SITEMAP_CAP = 5000
BULK_COUNT = int(os.environ.get("SEO_SITEMAP_LISTINGS", str(SITEMAP_CAP)))
BULK_SELLERS = 10


class Doc(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.titles: list[str] = []
        self.metas: list[dict[str, str]] = []
        self.links: list[dict[str, str]] = []
        self._in_title = False
        self._svg_depth = 0

    def handle_starttag(self, tag: str, attrs: list) -> None:
        a = {k: (v or "") for k, v in attrs}
        if tag == "svg":
            self._svg_depth += 1
        elif tag == "title" and self._svg_depth == 0:
            self._in_title = True
            self.titles.append("")
        elif tag == "meta":
            self.metas.append(a)
        elif tag == "link":
            self.links.append(a)

    def handle_endtag(self, tag: str) -> None:
        if tag == "svg" and self._svg_depth:
            self._svg_depth -= 1
        elif tag == "title":
            self._in_title = False

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.titles[-1] += data

    def meta(self, key: str) -> list[str]:
        return [
            m.get("content", "")
            for m in self.metas
            if m.get("name") == key or m.get("property") == key
        ]

    def og(self) -> list[dict[str, str]]:
        return [m for m in self.metas if m.get("property", "").startswith("og:")]

    def canonicals(self) -> list[str]:
        return [lk.get("href", "") for lk in self.links if lk.get("rel", "") == "canonical"]


class Fetched:
    def __init__(self, path: str, resp: httpx.Response) -> None:
        self.path = path
        self.resp = resp
        self.doc = Doc()
        self.doc.feed(resp.text)


def _x(world: World) -> dict:
    return world.state.extra.setdefault("seo", {})


def _sub(world: World, text: str) -> str:
    x = _x(world)
    tokens = {
        "[site]": SITE,
        "[listing]": x.get("listing_id", ""),
        "[listing title]": x.get("listing_title", ""),
        "[shop]": x.get("shop_id", ""),
        "[category]": x.get("category_id", ""),
        "[category name]": x.get("category_name", ""),
    }
    for token, value in tokens.items():
        text = text.replace(token, value)
    return text


def _gateway(path: str, body: dict, token: str | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{SETTINGS.gateway_url.rstrip('/')}{path}", json=body, headers=headers, timeout=30
    )


def _fetch(
    world: World,
    app: str,
    path: str,
    *,
    token: str | None = None,
    host: str | None = None,
) -> Fetched:
    base, cookie = APPS[app]
    headers = {"User-Agent": CRAWLER_UA}
    if token:
        headers["Cookie"] = f"{cookie}={token}"
    if host:
        headers["Host"] = host
    with httpx.Client(follow_redirects=False, timeout=60) as client:
        resp = client.get(f"{base}{_sub(world, path)}", headers=headers)
    return Fetched(path, resp)


def _store(world: World, fetched: list[Fetched]) -> None:
    _x(world)["fetched"] = fetched


def _all(world: World) -> list[Fetched]:
    fetched = _x(world).get("fetched")
    assert fetched, "no page was fetched"
    return fetched


def _new_seller(prefix: str = "seo_seller") -> tuple[str, str]:
    token = AuthService().register(fake.unique_username(prefix), SETTINGS.seed_password, "seller")
    return token, ues.user_id_of(token)


def _create_listing(
    token: str,
    *,
    title: str | None = None,
    description: str = "Sản phẩm seed tự động cho SEO E2E.",
    status: str = "published",
    image_keys: list[str] | None = None,
) -> tuple[str, str]:
    svc = ListingService(token=token)
    title = title or f"SEO {uuid.uuid4().hex[:10]}"
    listing = Listing(title=title, price=250_000, stock=10, status=status, description=description)
    listing_id = svc.create_listing(listing)
    if image_keys:
        current = svc.get_listing(listing_id)
        current["imageKeys"] = image_keys
        svc.post(f"{LISTING_RPC}UpdateListing", {"listing": current})
    return listing_id, title


def _remember_listing(world: World, listing_id: str, title: str, seller_id: str) -> None:
    _x(world).update(listing_id=listing_id, listing_title=title, seller_id=seller_id)


def _wait_hidden(listing_id: str) -> None:
    deadline = time.time() + 60
    while time.time() < deadline:
        resp = _gateway(f"{LISTING_RPC}GetListing", {"id": listing_id})
        if resp.status_code != 200:
            return
        time.sleep(1)
    raise AssertionError(f"listing {listing_id} is still readable anonymously")


def _make_unpublished(world: World, status: str) -> tuple[str, str]:
    token, seller_id = _new_seller()
    if status == "draft":
        listing_id, title = _create_listing(token, status="draft")
    else:
        listing_id, title = _create_listing(token)
        if status == "banned":
            resp = _gateway(
                f"{LISTING_RPC}ModerateListing",
                {
                    "listingId": listing_id,
                    "decision": "MODERATION_DECISION_BAN",
                    "reasonCode": "other",
                },
                aud.admin_token(world),
            )
            assert resp.status_code == 200, f"ModerateListing {resp.status_code}: {resp.text}"
        elif status == "held":
            resp = ues.set_user_status(world, seller_id, "suspended", until=ues.in_future(3600))
            assert resp.status_code == 200, f"SetUserStatus {resp.status_code}: {resp.text}"
        else:
            raise AssertionError(f"unknown status {status}")
    _wait_hidden(listing_id)
    _remember_listing(world, listing_id, title, seller_id)
    return listing_id, title


@given("a published listing with an image")
def listing_with_image(world: World) -> None:
    token, seller_id = _new_seller()
    keys = [f"seo/{uuid.uuid4().hex[:8]}-{i}.png" for i in range(2)]
    listing_id, title = _create_listing(token, image_keys=keys)
    _remember_listing(world, listing_id, title, seller_id)
    _x(world)["image_keys"] = keys


@given("a published listing without an image")
def listing_without_image(world: World) -> None:
    token, seller_id = _new_seller()
    listing_id, title = _create_listing(token)
    _remember_listing(world, listing_id, title, seller_id)


@given("a published listing whose description is 400 characters over several lines")
def listing_long_description(world: World) -> None:
    lines = [f"dong {i} " + "mo ta san pham " * 3 for i in range(1, 9)]
    description = "\n".join(lines)[:400]
    assert len(description) == 400 and "\n" in description
    token, seller_id = _new_seller()
    listing_id, title = _create_listing(token, description=description)
    _remember_listing(world, listing_id, title, seller_id)
    _x(world)["description"] = description


@given(parsers.parse('a listing in status "{status}"'))
def listing_in_status(world: World, status: str) -> None:
    _make_unpublished(world, status)


@given(parsers.parse('a seller with the shop name "{name}" and a storefront banner'))
def seller_with_shop(world: World, name: str) -> None:
    token, seller_id = _new_seller("seo_shop")
    banner = f"https://cdn.e2e.example/seo/{uuid.uuid4().hex[:8]}-banner.png"
    resp = _gateway(
        f"{LISTING_RPC}UpsertStorefront",
        {
            "storefront": {
                "slug": f"seo-{uuid.uuid4().hex[:10]}",
                "displayName": name,
                "bannerUrl": banner,
            }
        },
        token,
    )
    assert resp.status_code == 200, f"UpsertStorefront {resp.status_code}: {resp.text}"
    _x(world).update(shop_id=seller_id, banner=banner)


@given("an existing category")
def existing_category(world: World) -> None:
    categories = ListingService().list_categories()
    assert categories, "the stack has no category"
    first = categories[0]
    _x(world).update(category_id=first["id"], category_name=first["name"])


@given("a signed-in buyer")
def signed_in_buyer(world: World) -> None:
    token = AuthService().register(
        fake.unique_username("seo_buyer"), SETTINGS.seed_password, "buyer"
    )
    _x(world)["token"] = token


@given("a signed-in seller")
def signed_in_seller(world: World) -> None:
    _x(world)["token"], _ = _new_seller()


@given("a signed-in admin")
def signed_in_admin(world: World) -> None:
    _x(world)["token"] = aud.admin_token(world)


@given("4 published listings of 2 sellers created one after another")
def four_listings_two_sellers(world: World) -> None:
    sellers = [_new_seller(), _new_seller()]
    ids: list[str] = []
    for i in range(4):
        token, _ = sellers[i % 2]
        listing_id, _title = _create_listing(token)
        ids.append(listing_id)
        time.sleep(1.2)
    _x(world).update(listing_ids=ids, shop_ids=[s[1] for s in sellers])


@given("a published listing and a banned listing and a held listing")
def published_banned_held(world: World) -> None:
    token, _ = _new_seller()
    published, _t = _create_listing(token)
    banned, _t = _make_unpublished(world, "banned")
    held, _t = _make_unpublished(world, "held")
    _x(world).update(published_id=published, banned_id=banned, held_id=held)


@given("5000 published listings are seeded")
def seed_bulk(world: World) -> None:
    tokens = [_new_seller("seo_bulk")[0] for _ in range(BULK_SELLERS)]
    started = time.monotonic()

    def make(i: int) -> str:
        return _create_listing(tokens[i % BULK_SELLERS], title=f"SEO bulk {uuid.uuid4().hex[:8]}")[
            0
        ]

    with ThreadPoolExecutor(max_workers=16) as pool:
        ids = list(pool.map(make, range(BULK_COUNT)))
    _x(world).update(bulk_ids=ids, seed_seconds=time.monotonic() - started)


@when(parsers.parse('an anonymous crawler fetches "{path}" on the {app}'))
def anonymous_fetch(world: World, path: str, app: str) -> None:
    _store(world, [_fetch(world, app, path)])


@when(parsers.parse('a crawler fetches "{path}" on the storefront claiming the Host "{host}"'))
def forged_host_fetch(world: World, path: str, host: str) -> None:
    _store(world, [_fetch(world, "storefront", path, host=host)])


def _quoted(text: str) -> list[str]:
    return re.findall(r'"([^"]+)"', text)


@when(parsers.re(r"an anonymous crawler fetches each of (?P<paths>.+) on the (?P<app>[a-z ]+)$"))
def anonymous_fetch_each(world: World, paths: str, app: str) -> None:
    _store(world, [_fetch(world, app, p) for p in _quoted(paths)])


@when(parsers.parse('the signed-in {role} fetches "{path}" on the {app}'))
def signed_in_fetch(world: World, role: str, path: str, app: str) -> None:
    _store(world, [_fetch(world, app, path, token=_x(world)["token"])])


@when(
    parsers.re(
        r"the signed-in (?P<role>\w+) fetches each of (?P<paths>.+) on the (?P<app>[a-z ]+)$"
    )
)
def signed_in_fetch_each(world: World, role: str, paths: str, app: str) -> None:
    token = _x(world)["token"]
    _store(world, [_fetch(world, app, p, token=token) for p in _quoted(paths)])


def _locs(xml: str) -> list[str]:
    return re.findall(r"<loc>\s*([^<]+?)\s*</loc>", xml)


def _fetch_sitemap_until(world: World, ready, limit_s: float) -> None:
    deadline = time.time() + limit_s
    last: Fetched | None = None
    started = time.monotonic()
    while time.time() < deadline:
        started = time.monotonic()
        last = _fetch(world, "storefront", "/sitemap.xml")
        locs = _locs(last.resp.text) if last.resp.status_code == 200 else []
        if ready(locs):
            _x(world)["sitemap_seconds"] = time.monotonic() - started
            _store(world, [last])
            return
        time.sleep(2)
    _store(world, [last] if last else [])
    raise AssertionError("the sitemap never listed the seeded listings")


@when('the crawler fetches "/sitemap.xml" once the four listings appear')
def sitemap_four(world: World) -> None:
    wanted = {f"{SITE}/listing/{i}" for i in _x(world)["listing_ids"]}
    _fetch_sitemap_until(world, lambda locs: wanted <= set(locs), SITEMAP_WAIT_S)


@when('the crawler fetches "/sitemap.xml" once the published listing appears')
def sitemap_published(world: World) -> None:
    wanted = f"{SITE}/listing/{_x(world)['published_id']}"
    _fetch_sitemap_until(world, lambda locs: wanted in locs, SITEMAP_WAIT_S)


@when('the crawler fetches "/sitemap.xml" once 5000 listing URLs appear')
def sitemap_bulk(world: World) -> None:
    want = min(BULK_COUNT, SITEMAP_CAP)
    seen: list[int] = []

    def stable(locs: list[str]) -> bool:
        listings = [u for u in locs if u.startswith(f"{SITE}/listing/")]
        seen.append(len(listings))
        return (
            len(listings) >= want
            and len(seen) >= 2
            and seen[-1] == seen[-2]
            and len(set(listings)) == len(listings)
        )

    _fetch_sitemap_until(world, stable, 600)


@when(parsers.parse('a visitor opens "{a}" and "{b}" in the browser'))
def visitor_opens(world: World, a: str, b: str) -> None:
    seen = []
    for path in (a, b):
        world.page.goto(f"{SITE}{path}", wait_until="load", timeout=timeouts.NAVIGATION)
        seen.append((path, world.page.title(), world.page.locator("body").inner_text()))
    _x(world)["browser_pages"] = seen


@then(parsers.parse("the response status is {status:d}"))
@then(parsers.parse("each response status is {status:d}"))
def status_is(world: World, status: int) -> None:
    for f in _all(world):
        assert f.resp.status_code == status, f"{f.path}: HTTP {f.resp.status_code}"


@then(parsers.parse('the response content type is "{expected}"'))
def content_type_is(world: World, expected: str) -> None:
    for f in _all(world):
        ctype = f.resp.headers.get("content-type", "")
        assert ctype.startswith(expected), f"{f.path}: content-type {ctype!r}"


@then(parsers.parse('the title is "{expected}"'))
def title_is(world: World, expected: str) -> None:
    for f in _all(world):
        assert f.doc.titles, f"{f.path}: no <title>"
        assert f.doc.titles[0].strip() == _sub(world, expected), f.doc.titles


@then(parsers.parse('the title ends with exactly one "{suffix}"'))
def title_one_suffix(world: World, suffix: str) -> None:
    for f in _all(world):
        assert f.doc.titles, f"{f.path}: no <title>"
        title = f.doc.titles[0].strip()
        assert title.endswith(suffix) and title.count(suffix) == 1, title


@then("the meta description is non-empty Vietnamese text")
def description_vietnamese(world: World) -> None:
    for f in _all(world):
        values = f.doc.meta("description")
        assert len(values) == 1 and values[0].strip(), f"{f.path}: description {values}"
        assert VIETNAMESE.search(values[0]), values[0]


@then(parsers.parse('the meta description is "{expected}"'))
def description_is(world: World, expected: str) -> None:
    for f in _all(world):
        assert f.doc.meta("description") == [expected], f.doc.meta("description")


@then(parsers.parse('the canonical link is "{expected}"'))
def canonical_is(world: World, expected: str) -> None:
    for f in _all(world):
        assert f.doc.canonicals() == [_sub(world, expected)], f.doc.canonicals()


@then(parsers.parse('the canonical link and og:url start with "{prefix}"'))
def canonical_and_og_url_prefix(world: World, prefix: str) -> None:
    for f in _all(world):
        values = f.doc.canonicals() + f.doc.meta("og:url")
        assert len(values) == 2, f"canonical/og:url missing: {values}"
        assert all(v.startswith(_sub(world, prefix)) for v in values), values


@then(parsers.parse('neither the canonical link nor og:url contains "{text}"'))
def canonical_and_og_url_without(world: World, text: str) -> None:
    for f in _all(world):
        values = f.doc.canonicals() + f.doc.meta("og:url")
        assert values, "canonical/og:url missing"
        assert not [v for v in values if text in v], values


@then(parsers.parse('the meta tag {prop} is "{expected}"'))
def meta_is(world: World, prop: str, expected: str) -> None:
    for f in _all(world):
        assert f.doc.meta(prop) == [_sub(world, expected)], f"{prop}: {f.doc.meta(prop)}"


@then("the meta tag og:image is an absolute URL of the first image")
def og_image_first(world: World) -> None:
    first = _x(world)["image_keys"][0].rsplit("/", 1)[-1]
    second = _x(world)["image_keys"][1].rsplit("/", 1)[-1]
    for f in _all(world):
        values = f.doc.meta("og:image")
        assert len(values) == 1, f"og:image: {values}"
        assert re.match(r"^https?://", values[0]), values[0]
        assert first in values[0] and second not in values[0], values[0]


@then("the meta tag og:image is the storefront banner URL")
def og_image_banner(world: World) -> None:
    for f in _all(world):
        assert f.doc.meta("og:image") == [_x(world)["banner"]], f.doc.meta("og:image")


@then("the page has no og meta tag and no canonical link")
def no_og_no_canonical(world: World) -> None:
    for f in _all(world):
        assert f.resp.text.strip(), f"{f.path}: empty body"
        assert f.doc.titles, f"{f.path}: positive control failed, no <title> in the 404 page"
        assert not f.doc.og(), f"{f.path}: og tags {f.doc.og()}"
        assert not f.doc.canonicals(), f"{f.path}: canonical {f.doc.canonicals()}"


@then("the page has no og:image meta tag")
def no_og_image(world: World) -> None:
    for f in _all(world):
        assert f.doc.og(), f"{f.path}: positive control failed, no og tag at all"
        assert not f.doc.meta("og:image"), f.doc.meta("og:image")


@then("each response has no og meta tag")
def each_no_og(world: World) -> None:
    for f in _all(world):
        assert f.doc.titles, f"{f.path}: positive control failed, no <title>"
        assert not f.doc.og(), f"{f.path}: og tags {f.doc.og()}"


@then("the page has no canonical link")
def no_canonical(world: World) -> None:
    for f in _all(world):
        assert f.doc.titles, f"{f.path}: positive control failed, no <title>"
        assert not f.doc.canonicals(), f.doc.canonicals()


@then("the page has no noindex robots tag")
def no_noindex(world: World) -> None:
    for f in _all(world):
        assert f.doc.titles, f"{f.path}: positive control failed, no <title>"
        assert not [r for r in f.doc.meta("robots") if "noindex" in r.lower()], f.doc.meta("robots")


@then(parsers.parse('the robots meta is "{expected}"'))
@then(parsers.parse('each response robots meta is "{expected}"'))
def robots_meta_is(world: World, expected: str) -> None:
    for f in _all(world):
        assert f.doc.meta("robots") == [expected], f"{f.path}: robots {f.doc.meta('robots')}"


@then(parsers.parse('the referrer meta is "{expected}"'))
@then(parsers.parse('each response referrer meta is "{expected}"'))
def referrer_meta_is(world: World, expected: str) -> None:
    for f in _all(world):
        assert f.doc.meta("referrer") == [expected], f"{f.path}: referrer {f.doc.meta('referrer')}"


@then("the meta description and og:description start with the listing description")
def description_starts(world: World) -> None:
    head = _x(world)["description"].split("\n")[0][:20]
    for f in _all(world):
        for key in ("description", "og:description"):
            values = f.doc.meta(key)
            assert len(values) == 1 and values[0].startswith(head), f"{key}: {values}"


@then("the meta description and og:description hold at most 160 characters and no line break")
def description_cut(world: World) -> None:
    for f in _all(world):
        for key in ("description", "og:description"):
            (value,) = f.doc.meta(key)
            assert len(value) <= 160, f"{key}: {len(value)} characters"
            assert "\n" not in value and "\r" not in value, f"{key}: line break"


@then("the query text appears in no title or og meta tag")
def query_not_echoed(world: World) -> None:
    for f in _all(world):
        assert f.doc.titles, f"{f.path}: positive control failed, no <title>"
        texts = f.doc.titles + [m.get("content", "") for m in f.doc.og()]
        assert not [t for t in texts if "script" in t.lower()], texts


def _robots_lines(world: World) -> list[str]:
    (f,) = _all(world)
    return [ln.strip() for ln in f.resp.text.splitlines() if ln.strip()]


@then("robots.txt disallows each of the twelve private prefixes")
def robots_disallows(world: World) -> None:
    lines = _robots_lines(world)
    missing = [p for p in PRIVATE_PREFIXES if f"Disallow: {p}" not in lines]
    assert not missing, f"missing Disallow for {missing}"
    lowered = [ln.lower() for ln in lines]
    assert "user-agent: *" in lowered and "allow: /" in lowered, lines


@then(parsers.parse('the last line of robots.txt is "{expected}"'))
def robots_last(world: World, expected: str) -> None:
    assert _robots_lines(world)[-1] == _sub(world, expected), _robots_lines(world)


@then(parsers.parse('robots.txt has the line "{line}"'))
def robots_has(world: World, line: str) -> None:
    assert line in _robots_lines(world), _robots_lines(world)


def _sitemap_locs(world: World) -> list[str]:
    (f,) = _all(world)
    return _locs(f.resp.text)


@then("the sitemap holds the home URL and every category URL")
def sitemap_home_categories(world: World) -> None:
    locs = _sitemap_locs(world)
    assert locs.count(SITE) + locs.count(f"{SITE}/") >= 1, "home URL missing"
    categories = ListingService().list_categories()
    assert categories, "the stack has no category"
    missing = [c["id"] for c in categories if f"{SITE}/search?category={c['id']}" not in locs]
    assert not missing, f"category URLs missing: {missing}"


@then("the sitemap lists the four listings newest first")
def sitemap_newest_first(world: World) -> None:
    locs = _sitemap_locs(world)
    ids = _x(world)["listing_ids"]
    positions = [locs.index(f"{SITE}/listing/{i}") for i in reversed(ids)]
    assert positions == sorted(positions), f"not newest first: {positions}"


@then("the sitemap lists each of the two shops exactly once")
def sitemap_shops(world: World) -> None:
    locs = _sitemap_locs(world)
    for shop in _x(world)["shop_ids"]:
        assert locs.count(f"{SITE}/shop/{shop}") == 1, f"shop {shop}"


@then(parsers.parse('every sitemap location is absolute on "{origin}"'))
def sitemap_absolute(world: World, origin: str) -> None:
    locs = _sitemap_locs(world)
    assert locs, "empty sitemap"
    base = _sub(world, origin)
    bad = [u for u in locs if not (u == base or u.startswith(f"{base}/"))]
    assert not bad, f"not absolute on {base}: {bad[:5]}"


@then("the sitemap does not list the banned listing")
def sitemap_no_banned(world: World) -> None:
    assert f"{SITE}/listing/{_x(world)['banned_id']}" not in _sitemap_locs(world)


@then("the sitemap does not list the held listing")
def sitemap_no_held(world: World) -> None:
    assert f"{SITE}/listing/{_x(world)['held_id']}" not in _sitemap_locs(world)


@then("the sitemap lists no private prefix")
def sitemap_no_private(world: World) -> None:
    locs = _sitemap_locs(world)
    assert locs, "empty sitemap"
    bad = [
        u
        for u in locs
        for p in PRIVATE_PREFIXES
        if u.startswith(f"{SITE}{p.rstrip('/')}/") or u == f"{SITE}{p.rstrip('/')}"
    ]
    assert not bad, bad[:5]


@then("the sitemap holds exactly 5000 listing URLs")
def sitemap_exact_cap(world: World) -> None:
    listings = [u for u in _sitemap_locs(world) if u.startswith(f"{SITE}/listing/")]
    assert len(listings) == min(BULK_COUNT, SITEMAP_CAP), len(listings)
    assert len(set(listings)) == len(listings), "duplicate listing URLs"


@then("the cold-render duration is recorded in the run output")
def record_duration(world: World) -> None:
    x = _x(world)
    seconds = x["sitemap_seconds"]
    pages = math.ceil(min(BULK_COUNT, SITEMAP_CAP) / 50)
    print(
        f"SEO-36 sitemap: {BULK_COUNT} listings seeded in {x['seed_seconds']:.1f}s; "
        f"final /sitemap.xml fetch took {seconds:.2f}s (target 2s, not a gate); "
        f"expected SearchListings requests {pages} at page_size 50 "
        "(not observable from outside the gateway)"
    )
    assert seconds > 0


@then(
    parsers.parse(
        'no page text or title contains "{a}", "{b}" or "{c}"',
    )
)
def no_brand(world: World, a: str, b: str, c: str) -> None:
    pages = _x(world)["browser_pages"]
    assert pages and all(text.strip() for _, _, text in pages), "positive control: empty page"
    for path, title, text in pages:
        for brand in (a, b, c):
            assert brand not in title and brand not in text, f"{path}: {brand!r}"


@then("the icon link href of the page answers 200 with an image content type")
def favicon_served(world: World) -> None:
    (f,) = _all(world)
    icons = [
        lk.get("href", "")
        for lk in f.doc.links
        if "icon" in lk.get("rel", "").split() and lk.get("href")
    ]
    assert icons, "no <link rel=icon> on the page"
    resp = httpx.get(urljoin(f"{SITE}/", icons[0]), headers={"User-Agent": CRAWLER_UA}, timeout=30)
    assert resp.status_code == 200, f"{icons[0]}: HTTP {resp.status_code}"
    assert resp.headers.get("content-type", "").startswith("image/"), resp.headers
