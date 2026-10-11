"""Static assertions on the local compose infra (local-compose-infra). No docker: the compose files are parsed."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml
from pytest_bdd import then, when

ROOT = Path(__file__).resolve().parents[3].parent
INFRA = ROOT / "platform-core/infra/docker-compose.yaml"
SERVICES = ROOT / "docker-compose.services.yaml"
# Images allowed to float; listed by name so a new floating image fails the test. None today.
KNOWN_UNPINNED: set[str] = set()


@pytest.fixture
def lci():
    return {}


def _services(path: Path) -> dict:
    return yaml.safe_load(path.read_text())["services"]


@when("the healthchecks of the compose files are read")
def read_healthchecks(lci):
    lci["checks"] = {
        f"{p.name}:{name}": svc["healthcheck"]["test"]
        for p in (INFRA, SERVICES)
        for name, svc in _services(p).items()
        if "healthcheck" in svc
    }
    assert lci["checks"], "no healthchecks found"


@then("none uses CMD-SHELL with /dev/tcp, and the qdrant healthcheck is CMD with bash")
def no_sh_dev_tcp(lci):
    bad = [
        k
        for k, t in lci["checks"].items()
        if isinstance(t, list) and t[:1] == ["CMD-SHELL"] and "/dev/tcp" in " ".join(t)
    ]
    assert not bad, f"CMD-SHELL runs sh, which has no /dev/tcp: {bad}"
    qdrant = lci["checks"]["docker-compose.yaml:qdrant"]
    assert qdrant[:3] == ["CMD", "bash", "-c"], qdrant


@when("the depends_on of the root compose services are read")
def read_depends(lci):
    lci["deps"] = {
        name: svc["depends_on"]
        for name, svc in _services(SERVICES).items()
        if isinstance(svc.get("depends_on"), dict) and "qdrant" in svc["depends_on"]
    }
    assert lci["deps"], "no root compose service depends on qdrant"


@then("every map-form dependency on qdrant has condition service_healthy")
def qdrant_healthy(lci):
    bad = {
        n: d["qdrant"]
        for n, d in lci["deps"].items()
        if d["qdrant"].get("condition") != "service_healthy"
    }
    assert not bad, bad


@when("the images of platform-core/infra/docker-compose.yaml are read")
def read_images(lci):
    lci["images"] = {name: svc["image"] for name, svc in _services(INFRA).items() if "image" in svc}
    assert lci["images"], "no images found"


@then(
    "each has a tag that is not latest except the known exceptions, and minio-init runs "
    "bitnamilegacy/minio-client"
)
def compose_pinned(lci):
    wrong = []
    for name, image in lci["images"].items():
        repo, _, tag = image.rpartition(":") if ":" in image.split("/")[-1] else (image, "", "")
        if (not tag or tag == "latest" or repo == "minio/mc") and image not in KNOWN_UNPINNED:
            wrong.append(f"{name}: {image}")
    assert not wrong, wrong
    assert lci["images"]["minio-init"].startswith("bitnamilegacy/minio-client:")
