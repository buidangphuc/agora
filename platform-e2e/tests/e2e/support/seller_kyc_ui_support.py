"""Run-time fixtures and fault toggles for the seller KYC UI scenarios (change seller-kyc-ui).

Files are generated in memory when a scenario selects them (nothing is committed): a valid PNG header
or PDF/JPEG magic, a unique marker that must never reach the seller app's own requests, and padding
up to the exact size. Faults are real container stops restored by a world cleanup.
"""

from __future__ import annotations

import os
import time
import uuid

import httpx

from tests.e2e.support import kyc_documents_support as kyc
from tests.e2e.support import plp_stack as stack

MIB = 1024 * 1024
PNG_MAGIC = b"\x89PNG\r\n\x1a\n"
JPG_MAGIC = b"\xff\xd8\xff\xe0"
PDF_MAGIC = b"%PDF-1.4\n"
GIF_MAGIC = b"GIF89a"
KINDS = {
    "PNG": (PNG_MAGIC, "image/png", "png"),
    "JPG": (JPG_MAGIC, "image/jpeg", "jpg"),
    "PDF": (PDF_MAGIC, "application/pdf", "pdf"),
    "GIF": (GIF_MAGIC, "image/gif", "gif"),
}


def make_file(kind: str, name: str, size: int, marker: bytes) -> dict:
    """A Playwright set_input_files payload of exactly `size` bytes (0 gives an empty file)."""
    magic, mime, _ = KINDS[kind]
    body = (magic + marker)[:size] if size else b""
    body += b"\0" * (size - len(body))
    assert len(body) == size
    return {"name": name, "mimeType": mime, "buffer": body}


def valid_files(count: int, marker: bytes) -> list[dict]:
    order = ["PNG", "JPG", "PDF"]
    out = []
    for i in range(count):
        kind = order[i % len(order)]
        out.append(make_file(kind, f"giay-to-{i}.{KINDS[kind][2]}", 2048 + i, marker))
    return out


def new_marker() -> bytes:
    return uuid.uuid4().hex.encode() + uuid.uuid4().hex.encode()


def minio_container() -> str:
    return os.getenv("KYC_MINIO_CONTAINER", "agora-minio-1")


def stop_verification(world) -> None:  # noqa: ANN001
    """Stop team-verification; a world cleanup starts it again. Idempotent."""
    extra = world.state.extra
    if extra.get("skui_verification_stopped"):
        return
    stack.docker("stop", stack.container_name("team-verification"))
    extra["skui_verification_stopped"] = True

    def restore() -> None:
        if extra.get("skui_verification_stopped"):
            stack.docker("start", stack.container_name("team-verification"), check=False)
            extra["skui_verification_stopped"] = False
            stack.wait_settled("team-verification")

    world.add_cleanup(restore)


def stop_storage(world) -> None:  # noqa: ANN001
    extra = world.state.extra
    if extra.get("skui_storage_stopped"):
        return
    stack.docker("stop", minio_container())
    extra["skui_storage_stopped"] = True
    world.add_cleanup(lambda: start_storage(world))


def start_storage(world) -> None:  # noqa: ANN001
    extra = world.state.extra
    if not extra.get("skui_storage_stopped"):
        return
    stack.docker("start", minio_container(), check=False)
    extra["skui_storage_stopped"] = False
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            if httpx.get(f"{kyc.s3_endpoint()}/minio/health/live", timeout=3).status_code == 200:
                return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise TimeoutError("the KYC document storage did not come back")
