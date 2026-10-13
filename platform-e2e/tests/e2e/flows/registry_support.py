"""MLflow registry helpers for the job drivers (change recsys-auto-train).

Copied next to `driver.py` into /work by every job flow and imported by the driver INSIDE the
platform-recsys image, where it reaches MLflow at MLFLOW_TRACKING_URI (default http://mlflow:5050).
Standard library only for the REST reads; champions are seeded through the library's public `seed`
(py-modelregistry is installed in the image). It replaces the removed promotion levers and the
Redis registry:

- a fresh run promotes because the registry was reset (no champion) or holds a weak champion;
- a rejection is driven by a strong champion no candidate can match.

The registry is global to the stack, so a reset here is destructive for every other recsys scenario
(same as `MlflowRegistry.reset_registry` on the host).
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request

PREFIX = "recsys-"
ALS, GBDT, TWO_TOWER = "recsys-als", "recsys-gbdt", "recsys-two-tower"
EVAL_PROTOCOL = "leave-last-new-item-v1"
# Kept in step with src/api/mlflow_registry.py (WEAK / STRONG): a champion the candidate beats, and
# one nothing can match (ndcg, recall and coverage live in [0, 1]).
WEAK = {
    ALS: {"ndcg@10": 0.001, "coverage@10": 0.001},
    GBDT: {"ndcg@10": 0.001, "recall@10": 0.001},
    TWO_TOWER: {"recall@10": 0.001, "coverage@10": 0.001},
}
STRONG = {
    ALS: {"ndcg@10": 2.0, "coverage@10": 2.0},
    GBDT: {"ndcg@10": 2.0, "recall@10": 2.0},
    TWO_TOWER: {"recall@10": 2.0, "coverage@10": 2.0},
}


def base() -> str:
    return (os.environ.get("MLFLOW_TRACKING_URI") or "http://mlflow:5050").rstrip("/")


def _call(method: str, path: str, *, params: dict | None = None, body: dict | None = None) -> dict:
    url = f"{base()}{path}" + (f"?{urllib.parse.urlencode(params)}" if params else "")
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        url, data=data, method=method, headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:  # noqa: S310 - fixed stack URL
            raw = resp.read()
    except urllib.error.HTTPError as err:
        if err.code in (404, 400) and "alias" in path:
            return {}
        raise
    return json.loads(raw) if raw else {}


def registered_models() -> list[str]:
    names: list[str] = []
    token = None
    while True:
        params = {"max_results": 100, **({"page_token": token} if token else {})}
        data = _call("GET", "/api/2.0/mlflow/registered-models/search", params=params)
        names += [m["name"] for m in data.get("registered_models", [])]
        token = data.get("next_page_token")
        if not token:
            return sorted(n for n in names if n.startswith(PREFIX))


def reset() -> None:
    """Delete every `recsys-*` registered model: the next run has no champion and promotes."""
    for name in registered_models():
        _call("DELETE", "/api/2.0/mlflow/registered-models/delete", body={"name": name})


def seed_champion(kind: str, models: tuple[str, ...] = (ALS,)) -> None:
    """Seed a `weak` (candidate beats it) or `strong` (nothing beats it) champion for `models`."""
    from modelregistry import seed  # noqa: PLC0415 - only the job image has it

    os.environ.setdefault("MLFLOW_TRACKING_URI", base())
    metrics = WEAK if kind == "weak" else STRONG
    for model in models:
        lineage = {"data_version": "e2e-seed", "eval_protocol": EVAL_PROTOCOL}
        seed(model, metrics[model], lineage, f"e2e seeded {kind} champion")


def versions(name: str) -> list[dict]:
    out: list[dict] = []
    token = None
    while True:
        params = {"filter": f"name='{name}'", "max_results": 200}
        if token:
            params["page_token"] = token
        data = _call("GET", "/api/2.0/mlflow/model-versions/search", params=params)
        out += data.get("model_versions", [])
        token = data.get("next_page_token")
        if not token:
            return sorted(out, key=lambda v: int(v["version"]))


def champion_version(name: str) -> int | None:
    data = _call(
        "GET", "/api/2.0/mlflow/registered-models/alias", params={"name": name, "alias": "champion"}
    )
    return int(data["model_version"]["version"]) if data else None


def counts() -> dict[str, int]:
    """Highest version number per recsys model; the `before` mark of a run."""
    return {
        n: max((int(v["version"]) for v in versions(n)), default=0) for n in (ALS, GBDT, TWO_TOWER)
    }


def _coerce(value: str):
    try:
        return json.loads(value)
    except ValueError:
        return {"True": True, "False": False}.get(value, value)


def _unflatten(flat: dict) -> dict:
    out: dict = {}
    for key, value in flat.items():
        node = out
        *head, leaf = key.split(".")
        for part in head:
            child = node.get(part)
            if not isinstance(child, dict):
                child = node[part] = {}
            node = child
        node[leaf] = _coerce(value) if isinstance(value, str) else value
    return out


def _tags(version: dict) -> dict[str, str]:
    raw = version.get("tags") or []
    return dict(raw) if isinstance(raw, dict) else {t["key"]: t["value"] for t in raw}


def _meta(version: dict, model_version: str) -> dict:
    run = _call("GET", "/api/2.0/mlflow/runs/get", params={"run_id": version["run_id"]})["run"]
    data = run.get("data", {})
    tags = _tags(version)
    params = _unflatten({p["key"]: p["value"] for p in data.get("params", [])})
    try:
        sketches = _call(
            "GET",
            "/get-artifact",
            params={"path": "drift_sketches.json", "run_uuid": run["info"]["run_id"]},
        )
        params["distribution"] = sketches
    except (urllib.error.URLError, ValueError):
        pass  # a run without sketches (drift disabled, rejected before the stage)
    if tags.get("gate.reason"):
        params["gate_reason"] = tags["gate.reason"]
    metrics = {m["key"].replace("_at_", "@"): m["value"] for m in data.get("metrics", [])}
    if tags.get("lineage.eval_protocol"):
        metrics["eval_protocol"] = tags["lineage.eval_protocol"]
    return {
        "model_version": model_version,
        "registry_version": int(version["version"]),
        "status": tags.get("gate.outcome"),
        "metrics": metrics,
        "parameters": params,
    }


def snapshot(model_version: str | None, before: dict[str, int] | None = None) -> dict:
    """What the run left in MLflow, shaped like the old Redis registry snapshot.

    `models[<model_version>]` is the newest `recsys-als` version created after `before`,
    `models["gbdt-<model_version>"]` / `["two-tower-<model_version>"]` the newest of the others;
    `champion` is `model_version` when the `recsys-als` champion alias names that version, the
    registry version number as a string when it names another one, None without a champion.
    """
    before = before or {}
    models: dict[str, dict] = {}
    names = {ALS: "", GBDT: "gbdt-", TWO_TOWER: "two-tower-"}
    newest_als = None
    for name, prefix in names.items():
        fresh = [v for v in versions(name) if int(v["version"]) > before.get(name, 0)]
        if not fresh:
            continue
        label = f"{prefix}{model_version or 'run'}"
        models[label] = _meta(fresh[-1], model_version or "run")
        if name == ALS:
            newest_als = int(fresh[-1]["version"])
    champ = champion_version(ALS)
    champion = None if champ is None else (model_version if champ == newest_als else str(champ))
    gbdt_champ = champion_version(GBDT)
    return {
        "models": models,
        "champion": champion,
        "gbdt_champion": None if gbdt_champ is None else str(gbdt_champ),
    }
