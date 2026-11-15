"""Helpers for the frontend-legal-pages e2e features (change frontend-legal-pages).

Black box: pages are read over HTTP or in the browser, the ledger through the gateway
(PrivacyService.ListMyConsents), and "calls the gateway received" from the team-gateway
container log (docker logs, one `edge.request` line per call naming the procedure).
"""

from __future__ import annotations

import re
import time
from datetime import datetime, timezone

import httpx

from tests.e2e.support import pear_edge_support as pe

SLUGS = (
    "dieu-khoan-su-dung",
    "chinh-sach-bao-mat",
    "quy-che-hoat-dong",
    "chinh-sach-doi-tra",
    "hang-cam-kinh-doanh",
    "giai-quyet-khieu-nai",
    "dieu-khoan-nguoi-ban",
)
PLACEHOLDER_BODY = "NỘI DUNG DO BỘ PHẬN PHÁP CHẾ CUNG CẤP"
OPERATOR_PLACEHOLDER = "[DO BỘ PHẬN PHÁP CHẾ CUNG CẤP]"
RMA_LABEL = "Chính Sách Trả Hàng & Hoàn Tiền (RMA)"
RMA_TARGET = "/chinh-sach-doi-tra"

REGISTER_PROC = "AuthService/Register"
LIST_PROC = "PrivacyService/ListMyConsents"
RECORD_PROC = "PrivacyService/RecordConsent"
PRIVACY_PROC = "PrivacyService/"

_TAG = re.compile(r"<[^>]+>")
_COMMENT = re.compile(r"<!--.*?-->", re.S)
_H1 = re.compile(r"<h1[^>]*>(.*?)</h1>", re.S)
_VIETNAMESE = re.compile(
    r"[ăâđêôơưàáạảãằắặẳẵầấậẩẫèéẹẻẽềếệểễìíịỉĩòóọỏõồốộổỗờớợởỡùúụủũừứựửữỳýỵỷỹ]", re.I
)


def mark() -> str:
    """UTC timestamp usable as `docker logs --since`."""
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


def gateway_calls(procedure: str, since: str) -> int:
    """Gateway log lines naming `procedure` since `since`."""
    proc = pe.docker("logs", "--since", since, pe.gateway_container(), check=False)
    text = proc.stdout + proc.stderr
    return sum(1 for line in text.splitlines() if procedure in line)


def settle_gateway_log() -> None:
    time.sleep(2.0)


def get(base: str, path: str) -> httpx.Response:
    return httpx.get(f"{base.rstrip('/')}{path}", follow_redirects=False, timeout=30)


def visible_text(html: str) -> str:
    return re.sub(r"\s+", " ", _TAG.sub(" ", _COMMENT.sub("", html))).strip()


def h1_of(html: str) -> str:
    m = _H1.search(html)
    return visible_text(m.group(1)) if m else ""


def is_vietnamese(text: str) -> bool:
    return bool(text) and bool(_VIETNAMESE.search(text))


def version_after_label(html: str) -> str:
    m = re.search(r"Phiên bản\s*:?\s*([^\s<]+)", visible_text(html))
    return m.group(1) if m else ""


def sitemap_legal_urls(base: str) -> list[str]:
    resp = get(base, "/sitemap.xml")
    assert resp.status_code == 200, f"/sitemap.xml -> {resp.status_code}"
    locs = re.findall(r"<loc>\s*([^<\s]+)\s*</loc>", resp.text)
    return [u for u in locs if u.rstrip("/").rsplit("/", 1)[-1] in SLUGS]


def strip_multipart_fields(body: bytes, names: tuple[str, ...]) -> bytes:
    """Remove named parts from a multipart/form-data body (the forged-post scenario)."""
    first = body.split(b"\r\n", 1)[0]
    parts = body.split(first)
    kept = []
    for part in parts:
        if any(f'name="{n}"'.encode() in part for n in names):
            continue
        kept.append(part)
    return first.join(kept)
