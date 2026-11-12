#!/usr/bin/env python3
import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]
STATE_ROOT = Path(os.environ.get("AGORA_ENV_HOME", Path.home() / ".cache" / "agora-env"))
MAIN = "main"
MAIN_PROJECT = "mainx"
MAIN_TAG = "local"
MAIN_OVERLAYS = ["platform-e2e/compose/order-inventory.override.yaml", "platform-e2e/compose/rag-off.override.yaml"]
FEATURE_OVERLAYS = list(MAIN_OVERLAYS)
ENTRY = ["team-gateway"]
NOT_DEPS = {"TRUSTED_PROXIES", "CORS_ORIGINS", "JAEGER_UI_URL"}
OPTIONAL_HOSTS = {"otel-collector", "jaeger", "prometheus", "grafana", "kafka-ui"}
GATEWAY_FANOUT = re.compile(r"^UPSTREAM_")
FRONTEND_ARGS = {
    "team-frontend": {"NEXT_PUBLIC_GTM_ID": "GTM-E2E0001", "NEXT_PUBLIC_GATEWAY_URL": "gw", "NEXT_PUBLIC_MEDIA_BASE_URL": "media", "NEXT_PUBLIC_SELLER_APP_URL": "seller", "NEXT_PUBLIC_SITE_URL": "buyer"},
    "team-frontend-seller": {"NEXT_PUBLIC_GATEWAY_URL": "gw", "NEXT_PUBLIC_MEDIA_BASE_URL": "media", "NEXT_PUBLIC_BUYER_APP_URL": "buyer"},
    "team-frontend-admin": {"NEXT_PUBLIC_GATEWAY_URL": "gw"},
}
IMAGE_RE = re.compile(r"^(team-[a-z-]+|platform-[a-z-]+|agora-[a-z-]+|tei-fake|modelserve-router)$")


class Tagged:
    def __init__(self, tag, value):
        self.tag = tag
        self.value = value


yaml.add_representer(Tagged, lambda d, x: d.represent_sequence(x.tag, x.value))


def run(cmd, cwd=None, check=True, capture=False, env=None):
    res = subprocess.run(cmd, cwd=cwd, text=True, capture_output=capture, env=env)
    if check and res.returncode != 0:
        msg = (res.stderr or res.stdout or "").strip() if capture else ""
        sys.exit(f"agora-env: failed: {' '.join(map(str, cmd))}\n{msg}")
    return res


def out(cmd, cwd=None):
    return run(cmd, cwd=cwd, capture=True).stdout


def slug(name):
    s = re.sub(r"^(feat|fix|chore|asdlc|docs|refactor)/", "", name.strip())
    s = re.sub(r"[^a-z0-9-]+", "-", s.lower()).strip("-")
    if not s:
        sys.exit("agora-env: empty env name")
    return s


def state_dir(env):
    return STATE_ROOT / env


def load_state(env):
    p = state_dir(env) / "state.json"
    if not p.exists():
        sys.exit(f"agora-env: no env {env!r} (run `up` first)")
    return json.loads(p.read_text())


def save_state(st):
    d = state_dir(st["env"])
    d.mkdir(parents=True, exist_ok=True)
    (d / "state.json").write_text(json.dumps(st, indent=2) + "\n")


def compose_files(st):
    files = ["docker-compose.yaml", *st.get("overlays", [])]
    if st["env"] != MAIN:
        files.append(str(state_dir(st["env"]) / "override.yaml"))
    return [a for f in files for a in ("-f", f)]


def compose(st, *args, check=True, capture=False):
    return run(["docker", "compose", "-p", st["project"], *compose_files(st), *args], cwd=st["src"], check=check, capture=capture)


def config(src, overlays):
    files = [a for f in ["docker-compose.yaml", *overlays] for a in ("-f", f)]
    raw = out(["docker", "compose", *files, "--profile", "*", "config", "--format", "json"], cwd=src)
    return json.loads(raw)["services"]


def base_image(svc):
    return (svc.get("image") or "").rsplit(":", 1)[0]


def build_dir(src, svc):
    b = svc.get("build")
    if not b:
        return None
    ctx = Path(b["context"]).resolve()
    df = Path(b.get("dockerfile", "Dockerfile"))
    target = (ctx / df).parent if ctx == Path(src).resolve() else ctx
    try:
        return target.relative_to(Path(src).resolve()).as_posix()
    except ValueError:
        return None


def builder_of(services):
    by_image = {}
    for name, s in services.items():
        if s.get("build"):
            by_image[base_image(s)] = name
    return by_image


def image_users(services, builder):
    img = base_image(services[builder])
    return sorted(n for n, s in services.items() if base_image(s) == img)


def hosts(services):
    h = {}
    for name, s in services.items():
        h[name] = name
        if s.get("container_name"):
            h[s["container_name"]] = name
    return h


def deps_of(name, svc, hostmap, with_optional):
    found = set((svc.get("depends_on") or {}).keys())
    names = sorted(hostmap, key=len, reverse=True)
    for k, v in (svc.get("environment") or {}).items():
        if k in NOT_DEPS or v is None:
            continue
        if name == "team-gateway" and GATEWAY_FANOUT.match(k):
            continue
        for h in names:
            if re.search(rf"(?<![\w.-]){re.escape(h)}(?=[:/,]|$)", str(v)):
                found.add(hostmap[h])
    if not with_optional:
        found -= OPTIONAL_HOSTS
    found.discard(name)
    return found


def closure(services, start, with_optional=False):
    hostmap = hosts(services)
    seen, todo = set(), list(start)
    while todo:
        n = todo.pop()
        if n in seen or n not in services:
            continue
        seen.add(n)
        todo.extend(deps_of(n, services[n], hostmap, with_optional) - seen)
    return seen


def owners_from_slices(path):
    doc = yaml.safe_load(Path(path).read_text())
    return sorted({s["owner"] for s in doc.get("slices", []) if s.get("owner") and s["owner"] != "platform-core"})


def resolve_builders(services, names):
    by_dir = {}
    for n, s in services.items():
        d = build_dir(REPO, s) if s.get("build") else None
        if d:
            by_dir.setdefault(d, []).append(n)
    res = set()
    for n in names:
        if n in services and services[n].get("build"):
            res.add(n)
        elif n in by_dir:
            res.update(by_dir[n])
        else:
            sys.exit(f"agora-env: {n!r} is not a buildable compose service or repo dir")
    return sorted(res)


def listening_ports():
    ports = set()
    res = run(["lsof", "-nP", "-iTCP", "-sTCP:LISTEN"], check=False, capture=True)
    for line in res.stdout.splitlines()[1:]:
        m = re.search(r":(\d+) \(LISTEN\)", line)
        if m:
            ports.add(int(m.group(1)))
    return ports


def feature_port(p, slot):
    off = 30000 + 1000 * slot
    return p + off if p + off <= 65535 else p + 100 * (slot + 1)


def port_map(services, slot):
    pm = {}
    for s in services.values():
        for p in s.get("ports") or []:
            if p.get("published"):
                pub = int(str(p["published"]).split("-")[0])
                pm[pub] = feature_port(pub, slot)
    return pm


def rewrite_local(value, pm):
    def sub(m):
        return f"{m.group(1)}:{pm.get(int(m.group(2)), int(m.group(2)))}"
    return re.sub(r"\b(localhost|127\.0\.0\.1):(\d{2,5})\b", sub, value)


def frontend_args(name, pm):
    url = {
        "gw": f"http://localhost:{pm.get(8080, 8080)}",
        "media": f"http://localhost:{pm.get(9000, 9000)}/listing-images",
        "buyer": f"http://localhost:{pm.get(3000, 3000)}",
        "seller": f"http://localhost:{pm.get(3001, 3001)}",
    }
    return {k: url.get(v, v) for k, v in FRONTEND_ARGS.get(name, {}).items()}


def make_override(env, services, owners, pm):
    built = {base_image(services[o]) for o in owners}
    out_services = {}
    for name, s in services.items():
        o = {}
        if s.get("container_name"):
            o["container_name"] = f"{env}-{s['container_name']}"
            o["networks"] = {"default": {"aliases": [s["container_name"]]}}
        if s.get("ports"):
            o["ports"] = Tagged("!override", [
                f"127.0.0.1:{pm[int(str(p['published']).split('-')[0])]}:{p['target']}"
                for p in s["ports"] if p.get("published")
            ])
        env_vals = {k: rewrite_local(str(v), pm) for k, v in (s.get("environment") or {}).items() if v is not None}
        changed = {k: v for k, v in env_vals.items() if v != str(s["environment"][k])}
        if changed:
            o["environment"] = changed
        if name == "redpanda" and s.get("command"):
            o["command"] = [rewrite_local(c, pm).replace("external://localhost:", "external://127.0.0.1:") for c in s["command"]]
        if base_image(s) in built:
            o["image"] = f"{base_image(s)}:{env}"
            if s.get("build") and name in FRONTEND_ARGS:
                o["build"] = {"args": frontend_args(name, pm)}
        if o:
            out_services[name] = o
    return {"services": out_services}


def gen_stubs(src, name):
    if not name:
        return
    d = Path(src) / name
    if name.startswith("team-frontend"):
        if not (d / "node_modules").is_dir() or (d / "node_modules").is_symlink():
            run(["npm", "ci", "--no-audit", "--no-fund"], cwd=d)
        run(["npx", "buf", "generate"], cwd=d, env={**os.environ, "PATH": f"{d}/node_modules/.bin:{os.environ['PATH']}"})
        return
    gen = d / "buf.gen.yaml"
    if not gen.exists():
        return
    text = gen.read_text()
    text = text.replace("remote: buf.build/protocolbuffers/go", "local: protoc-gen-go")
    text = text.replace("remote: buf.build/grpc/go", "local: protoc-gen-go-grpc")
    text = re.sub(r"remote: buf\.build/connectrpc/go\S*", "local: protoc-gen-connect-go", text)
    with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as f:
        f.write(text)
    try:
        run(["buf", "generate", "--template", f.name], cwd=d, env={**os.environ, "PATH": f"{Path.home()}/go/bin:{os.environ['PATH']}"})
    finally:
        os.unlink(f.name)


def image_id(ref):
    res = run(["docker", "image", "inspect", "-f", "{{.Id}}", ref], check=False, capture=True)
    return res.stdout.strip() if res.returncode == 0 else None


def ids_in_use():
    res = run(["docker", "ps", "-a", "--format", "{{.ID}}"], capture=True)
    ids = res.stdout.split()
    if not ids:
        return set()
    return set(out(["docker", "inspect", "-f", "{{.Image}}", *ids]).split())


def drop_unused(ids):
    used = ids_in_use()
    for i in ids:
        if i and i not in used:
            run(["docker", "rmi", i], check=False, capture=True)
            print(f"removed old image {i[7:19]}")


def build(st, services, owners):
    tag = MAIN_TAG if st["env"] == MAIN else st["env"]
    old = {}
    for o in owners:
        ref = f"{base_image(services[o])}:{tag}"
        old[o] = image_id(ref)
        gen_stubs(st["src"], build_dir(st["src"], services[o]))
        t0 = time.time()
        args = [x for k, v in frontend_args(o, {}).items() for x in ("--build-arg", f"{k}={v}")] if st["env"] == MAIN else []
        compose(st, "build", *args, o)
        print(f"built {ref} in {int(time.time() - t0)}s")
    return [i for o, i in old.items() if i and i != image_id(f"{base_image(services[o])}:{tag}")]


def check_images(services, run_set, owners, env):
    built = {base_image(services[o]) for o in owners}
    missing = []
    for n in run_set:
        img = services[n].get("image")
        if not img or base_image(services[n]) in built:
            continue
        if ":" in img and image_id(img) is None and services[n].get("build"):
            missing.append(img)
    if missing:
        sys.exit(f"agora-env: main images missing for env {env}: {', '.join(sorted(set(missing)))} (run `promote` or add them as owners)")


def write_dc_wrapper(st):
    d = state_dir(st["env"])
    d.mkdir(parents=True, exist_ok=True)
    files = " ".join(f"'{a}'" for a in compose_files(st))
    w = d / "dc.sh"
    w.write_text(f"#!/bin/sh\ncd '{st['src']}' || exit 1\nexec docker compose -p '{st['project']}' {files} \"$@\"\n")
    w.chmod(0o755)
    return w


def cmd_plan(a):
    src = Path(a.src or REPO).resolve()
    services = config(src, FEATURE_OVERLAYS)
    names = list(a.services or [])
    if a.slices:
        names += owners_from_slices(a.slices)
    if not names:
        sys.exit("agora-env: give --slices or --services")
    owners = resolve_builders(services, names)
    start = set(ENTRY) | set(owners) | set(a.with_ or [])
    for o in owners:
        start.update(image_users(services, o))
    run_set = sorted(closure(services, start, a.with_optional))
    return src, services, owners, run_set


def cmd_plan_print(a):
    _, _, owners, run_set = cmd_plan(a)
    print("build:", " ".join(owners))
    print("run:  ", " ".join(run_set))


def cmd_up(a):
    env = slug(a.env)
    if env == MAIN:
        sys.exit("agora-env: main is updated with `promote`")
    src, services, owners, run_set = cmd_plan(a)
    prev = json.loads((state_dir(env) / "state.json").read_text()) if (state_dir(env) / "state.json").exists() else {}
    slot = prev.get("slot")
    if slot is None:
        taken = {json.loads(p.read_text()).get("slot") for p in STATE_ROOT.glob("*/state.json")}
        slot = next(i for i in range(10) if i not in taken)
    pm = port_map(services, slot)
    branch = out(["git", "rev-parse", "--abbrev-ref", "HEAD"], cwd=src).strip()
    st = {"env": env, "project": env, "src": str(src), "branch": a.branch or branch, "slot": slot,
          "owners": sorted(set(prev.get("owners", [])) | set(owners)), "services": sorted(set(prev.get("services", [])) | set(run_set)),
          "overlays": FEATURE_OVERLAYS + (a.overlay or []), "ports": {str(k): v for k, v in pm.items()}}
    if not prev:
        busy = listening_ports() & set(pm.values())
        if busy:
            sys.exit(f"agora-env: host ports already in use: {sorted(busy)}")
    d = state_dir(env)
    d.mkdir(parents=True, exist_ok=True)
    (d / "override.yaml").write_text(yaml.dump(make_override(env, services, st["owners"], pm), sort_keys=False))
    save_state(st)
    write_dc_wrapper(st)
    check_images(services, st["services"], st["owners"], env)
    old = [] if a.no_build else build(st, services, owners)
    compose(st, "up", "-d", "--no-build", *st["services"])
    if "team-gateway" in st["services"]:
        compose(st, "restart", "team-gateway", capture=True)
    drop_unused(old)
    print(f"env {env} up: project {env}, gateway http://localhost:{pm.get(8080)}, frontend http://localhost:{pm.get(3000)}")


def container(st, services, name):
    s = services.get(name) or {}
    cn = s.get("container_name")
    if st["env"] == MAIN:
        return cn or f"{st['project']}-{name}-1"
    return f"{st['env']}-{cn}" if cn else f"{st['project']}-{name}-1"


def image_ref(st, services, name):
    s = services.get(name) or {}
    built = {base_image(services[o]) for o in st.get("owners", []) if o in services}
    tag = st["env"] if st["env"] != MAIN and base_image(s) in built else MAIN_TAG
    return f"{base_image(s)}:{tag}"


def e2e_exports(st):
    services = config(st["src"], st.get("overlays", []))
    pm = {int(k): v for k, v in st.get("ports", {}).items()}
    port = lambda p: pm.get(p, p)
    P = st["project"]
    e = {
        "HEADLESS": "true", "BASE_URL": f"http://localhost:{port(3000)}", "GATEWAY_URL": f"http://localhost:{port(8080)}",
        "KAFKA_BROKERS": f"127.0.0.1:{port(19092)}",
        "SELLER_BASE_URL": f"http://localhost:{port(3001)}", "ADMIN_BASE_URL": f"http://localhost:{port(3002)}", "FLIPT_URL": f"http://localhost:{port(8083)}",
        "TEAM_AI_URL": f"http://localhost:{port(8001)}", "PROMETHEUS_URL": f"http://localhost:{port(9090)}",
        "MLFLOW_URL": f"http://localhost:{port(5050)}", "MINIO_URL": f"http://localhost:{port(9000)}",
        "SRM_OPENSEARCH_URL": f"http://localhost:{port(9200)}", "QDRANT_URL": f"http://localhost:{port(6333)}",
        "STACK_NETWORK": f"{P}_default", "MODELREGISTRY_COMPOSE_PROJECT": P, "UIF_COMPOSE_PROJECT": P,
        "DC_WRAPPER": str(write_dc_wrapper(st)), "WAREHOUSE_DB_PATH": "/data/analytics.duckdb",
        "ANALYTICS_VOLUME": f"{P}_analytics_data", "FSM_ANALYTICS_VOLUME": f"{P}_analytics_data",
        "FEATURESTORE_VOLUME": f"{P}_featurestore_data",
    }
    if st["env"] != MAIN:
        e["FSM_REDIS_PORT"] = str(port(6379))
    c = lambda n: container(st, services, n)
    for k in ("POSTGRES", "PLP_POSTGRES", "OIC_INV_POSTGRES", "AUDIT_POSTGRES"):
        e[f"{k}_CONTAINER"] = c("postgres")
    for k in ("REDPANDA", "PLP_REDPANDA"):
        e[f"{k}_CONTAINER"] = c("redpanda")
    e.update({"REDIS_CONTAINER": c("redis"), "JAEGER_CONTAINER": c("jaeger"), "MS_ROUTER_CONTAINER": c("modelserve-router"),
              "MLFLOW_CONTAINER": c("mlflow"), "GA4_RELAY_CONTAINER": c("ga4-relay"), "TEAM_ANALYTICS_CONTAINER": c("team-analytics")})
    for short in ("gateway", "identity", "domain", "payment", "frontend", "audit", "ai", "notification", "search"):
        e[f"{short.upper()}_CONTAINER"] = c(f"team-{short}")
    img = lambda n: image_ref(st, services, n)
    e.update({"IDENTITY_IMAGE": img("team-identity"), "IDENTITY_BOOT_IMAGE": img("team-identity"), "GATEWAY_BOOT_IMAGE": img("team-gateway"),
              "PAYMENT_BOOT_IMAGE": img("team-payment"), "SEARCH_BOOT_IMAGE": img("team-search"), "TS_IMAGE": img("team-search"),
              "ANALYTICS_IMAGE": img("team-analytics"), "RECSYS_IMAGE": img("platform-recsys-nearline"),
              "FEATURESTORE_IMAGE": img("featurestore-job") if "featurestore-job" in services else "platform-featurestore:local"})
    for n in services:
        if n.startswith("team-") and not n.endswith("-migrate"):
            k = n.upper().replace("-", "_")
            for pre in ("PLP", "OIC_INV"):
                e[f"{pre}_{k}_CONTAINER"] = c(n)
                e[f"{pre}_{k}_IMAGE"] = img(n)
            e[f"UIF_{k}_CONTAINER"] = c(n)
    return e


def cmd_e2e_env(a):
    st = load_state(slug(a.env) if a.env != MAIN else MAIN)
    for k, v in e2e_exports(st).items():
        print(f"export {k}='{v}'")


def cmd_e2e(a):
    st = load_state(slug(a.env) if a.env != MAIN else MAIN)
    args = list(a.pytest)
    if args and args[0] == "--":
        args = args[1:]
    if not args and not a.all:
        sys.exit("agora-env: give test paths, or --all for the full suite")
    wt = Path(a.wt or st["src"]).resolve()
    main_tree = Path(out(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], cwd=REPO).strip()).parent
    cands = [Path(os.environ["AGORA_E2E_PYTHON"])] if os.environ.get("AGORA_E2E_PYTHON") else []
    cands += [r / "platform-e2e" / ".venv" / "bin" / "python" for r in (wt, REPO, main_tree)]
    py = next((c for c in cands if c.exists()), None)
    if not py:
        sys.exit("agora-env: no platform-e2e venv found (set AGORA_E2E_PYTHON)")
    env = {**os.environ, **e2e_exports(st)}
    sys.exit(subprocess.run([str(py), "-m", "pytest", "-q", "-p", "no:cacheprovider", *args], cwd=wt / "platform-e2e", env=env).returncode)


def cmd_dc(a):
    st = load_state(slug(a.env) if a.env != MAIN else MAIN)
    args = a.args[1:] if a.args and a.args[0] == "--" else a.args
    sys.exit(compose(st, *args, check=False).returncode)


def env_images(env):
    res = out(["docker", "images", "--format", "{{.Repository}}:{{.Tag}}"])
    return sorted(r for r in res.split() if r.endswith(f":{env}") and IMAGE_RE.match(r.rsplit(":", 1)[0]))


def cmd_down(a):
    env = slug(a.env)
    if env in (MAIN, MAIN_PROJECT):
        sys.exit("agora-env: refusing to remove main")
    p = state_dir(env) / "state.json"
    if p.exists():
        compose(load_state(env), "down", "-v", "--remove-orphans", check=False)
    else:
        run(["docker", "compose", "-p", env, "down", "-v", "--remove-orphans"], check=False)
    label = f"label=com.docker.compose.project={env}"
    left = out(["docker", "ps", "-aq", "--filter", label]).split()
    if left:
        run(["docker", "rm", "-f", *left], check=False, capture=True)
    vols = out(["docker", "volume", "ls", "-q", "--filter", label]).split()
    if vols:
        run(["docker", "volume", "rm", *vols], check=False, capture=True)
    imgs = env_images(env)
    if imgs:
        run(["docker", "rmi", *imgs], check=False)
    shutil.rmtree(state_dir(env), ignore_errors=True)
    print(f"env {env} removed ({len(imgs)} images)")


def export_main(sha):
    src = STATE_ROOT / "main-src"
    if src.exists():
        shutil.rmtree(src)
    src.mkdir(parents=True)
    with tempfile.NamedTemporaryFile(suffix=".tar", delete=False) as f:
        run(["git", "archive", "--format=tar", "-o", f.name, sha], cwd=REPO)
    with tarfile.open(f.name) as t:
        t.extractall(src, filter="tar")
    os.unlink(f.name)
    return src


def changed_files(old, new):
    return out(["git", "diff", "--name-only", f"{old}..{new}"], cwd=REPO).split()


def deployed_owners(services, owners, existing, has_image, src=None):
    by_image = {}
    key = (lambda n: (build_dir(src, services[n]) != n, n)) if src else (lambda n: n)
    for o in sorted(owners, key=key):
        img = base_image(services[o])
        if set(image_users(services, o)) & existing or has_image(img):
            by_image.setdefault(img, o)
    return sorted(by_image.values())


def cmd_promote(a):
    run(["git", "fetch", "-q", "origin"], cwd=REPO)
    new = out(["git", "rev-parse", a.ref], cwd=REPO).strip()
    p = state_dir(MAIN) / "state.json"
    prev = json.loads(p.read_text()) if p.exists() else {}
    old = a.since or prev.get("deployed")
    if not old:
        sys.exit("agora-env: first promote needs --since <sha the running main was built from>")
    src = export_main(new)
    st = {"env": MAIN, "project": MAIN_PROJECT, "src": str(src), "branch": "main", "slot": None,
          "overlays": prev.get("overlays", MAIN_OVERLAYS) if not a.overlay else MAIN_OVERLAYS + a.overlay, "ports": {}, "owners": []}
    services = config(src, st["overlays"])
    files = changed_files(old, new)
    owners = set()
    for n, s in services.items():
        d = build_dir(src, s) if s.get("build") else None
        if d and any(f.startswith(d + "/") for f in files):
            owners.add(n)
    owners.update(resolve_builders(services, a.services or []))
    if any(f.startswith("platform-core/packages/proto/") for f in files):
        owners.update(n for n, s in services.items() if s.get("build") and (Path(src) / (build_dir(src, s) or "-") / "buf.gen.yaml").exists())
    existing = set(out(["docker", "ps", "-a", "--filter", f"label=com.docker.compose.project={MAIN_PROJECT}", "--format", '{{.Label "com.docker.compose.service"}}']).split())
    running = set(out(["docker", "ps", "--filter", f"label=com.docker.compose.project={MAIN_PROJECT}", "--format", '{{.Label "com.docker.compose.service"}}']).split())
    owners = deployed_owners(services, owners, existing, lambda img: image_id(f"{img}:{MAIN_TAG}") is not None, src)
    compose_changed = any(f.endswith((".yaml", ".yml")) and ("docker-compose" in f or f.startswith("platform-e2e/compose/")) for f in files)
    print(f"promote {old[:8]}..{new[:8]}: build {' '.join(owners) or '-'}{'; compose changed' if compose_changed else ''}")
    if a.dry_run:
        return
    save_state(st)
    write_dc_wrapper(st)
    olds = build(st, services, owners) if owners else []
    touched = sorted({u for o in owners for u in image_users(services, o)} & existing)
    for t in [x for x in touched if x.endswith("-migrate")]:
        compose(st, "up", "--no-build", "--no-deps", "--force-recreate", t, check=False)
    up = [x for x in touched if not x.endswith("-migrate") and x in running]
    stopped = [x for x in touched if not x.endswith("-migrate") and x not in running]
    if up:
        compose(st, "up", "-d", "--no-build", "--no-deps", "--force-recreate", *up)
    if stopped:
        compose(st, "up", "--no-start", "--no-build", "--no-deps", "--force-recreate", *stopped)
    if compose_changed:
        rest = sorted(running - set(up) - {"redpanda-init"})
        if rest:
            compose(st, "up", "-d", "--no-build", "--no-deps", *rest)
    if (up or compose_changed) and "team-gateway" in running and "team-gateway" not in up:
        run(["docker", "restart", container(st, services, "team-gateway")], capture=True)
    drop_unused(olds)
    st["deployed"] = new
    save_state(st)
    print(f"main at {new[:8]}; recreated {' '.join(up) or '-'}; refreshed stopped {' '.join(stopped) or '-'}")


def cmd_gc(a):
    live = {p.parent.name for p in STATE_ROOT.glob("*/state.json")}
    used = ids_in_use()
    removed = []
    for line in out(["docker", "images", "--format", "{{.Repository}}:{{.Tag}} {{.ID}}"]).splitlines():
        ref, iid = line.split()
        repo, tag = ref.rsplit(":", 1)
        if not IMAGE_RE.match(repo) or tag == MAIN_TAG or tag in live or tag == "<none>":
            continue
        full = image_id(ref)
        if full in used:
            continue
        if a.apply:
            run(["docker", "rmi", ref], check=False, capture=True)
        removed.append(ref)
    print(("removed" if a.apply else "would remove") + f" images: {' '.join(removed) or '-'}")
    if a.apply:
        run(["docker", "builder", "prune", "-f", "--filter", f"until={a.keep}"])
    else:
        print(f"would prune build cache older than {a.keep}")
    branches = set(out(["git", "for-each-ref", "--format=%(refname:short)", "refs/heads"], cwd=REPO).split())
    for env in sorted(live - {MAIN}):
        b = load_state(env).get("branch")
        if b and b not in branches:
            print(f"env {env}: branch {b} is gone -> agora_env.py down {env}")


def cmd_ls(a):
    for p in sorted(STATE_ROOT.glob("*/state.json")):
        st = json.loads(p.read_text())
        if a.porcelain:
            if st["env"] != MAIN:
                print(f"{st['env']} {st.get('branch') or '-'}")
            continue
        pm = st.get("ports") or {}
        print(f"{st['env']:<24} project={st['project']:<20} branch={st.get('branch')} gateway=:{pm.get('8080', 8080)} owners={','.join(st.get('owners', [])) or '-'}")


GITOPS = REPO / "platform-gitops"
GITEA = "http://ci:ci-pass-123@gitea.localtest.me/ci/platform-gitops.git"
REGISTRY = "localhost:5001"
INFRA_FILES = {
    "postgres": "postgres/postgres.yaml", "redpanda": "infra/redpanda.yaml", "redis": "infra/valkey.yaml",
    "flipt": "infra/flipt.yaml", "opensearch": "infra/opensearch.yaml", "qdrant": "infra/qdrant.yaml", "minio": "infra/minio.yaml",
}
K8S_HOST = {"redis": "valkey"}


def preview_hosts(env):
    return f"e2e-{env}.localtest.me", f"api.e2e-{env}.localtest.me"


def kube(*args, check=True, capture=True):
    return run(["kubectl", "--context", "kind-marketplace", *args], check=check, capture=capture)


def ensure_cluster():
    if "marketplace" not in out(["kind", "get", "clusters"]).split():
        print("kind cluster missing: running make -C deploy preview-cluster-up (several minutes)")
        run(["make", "-C", str(REPO / "deploy"), "preview-cluster-up"])
    else:
        run(["bash", str(REPO / "deploy" / "gitea" / "publish-gitops.sh")], capture=True)
        kube("apply", "-f", str(REPO / "deploy" / "argocd" / "preview-root-app.yaml"))


def k8s_value(v, services, fe, api):
    v = str(v)
    for name, s in services.items():
        cn = s.get("container_name")
        if cn:
            v = re.sub(rf"(?<![\w.-]){re.escape(cn)}(?![\w-])", name, v)
    for h, k in K8S_HOST.items():
        v = re.sub(rf"(?<![\w.-]){h}(?=:\d)", k, v)
    v = re.sub(r"([a-z0-9-]+)\.marketplace\.svc", r"\1", v)
    return v.replace("http://localhost:3000", f"http://{fe}").replace("http://localhost:8080", f"http://{api}")


def push_image(ref, repo):
    iid = image_id(ref)
    if not iid:
        sys.exit(f"agora-env: image {ref} not found")
    tag = f"sha-{iid.split(':')[-1][:12]}"
    target = f"{REGISTRY}/{repo}:{tag}"
    run(["docker", "tag", ref, target])
    run(["docker", "push", "-q", target], capture=True)
    run(["docker", "rmi", target], check=False, capture=True)
    return tag


def preview_frontend(src, env, fe, api):
    d = Path(src) / "team-frontend"
    gen_stubs(src, "team-frontend")
    ref = f"team-frontend:{env}-preview"
    old = image_id(ref)
    args = {"NEXT_PUBLIC_GTM_ID": "GTM-E2E0001", "NEXT_PUBLIC_GATEWAY_URL": f"http://{api}",
            "NEXT_PUBLIC_MEDIA_BASE_URL": f"http://{api}/listing-images", "NEXT_PUBLIC_SELLER_APP_URL": ""}
    cmd = ["docker", "build", "-q", "-t", ref]
    for k, v in args.items():
        cmd += ["--build-arg", f"{k}={v}"]
    run(cmd + [str(d)], capture=True)
    if old and old != image_id(ref):
        drop_unused([old])
    return ref


def gitea_write(env, files):
    with tempfile.TemporaryDirectory() as w:
        run(["git", "clone", "-q", GITEA, w], capture=True)
        d = Path(w) / "envs" / "e2e" / "initiatives" / env
        if d.exists():
            shutil.rmtree(d)
        if files:
            d.mkdir(parents=True)
            for name, doc in files.items():
                (d / name).write_text(yaml.dump(doc, sort_keys=False, width=1000))
        run(["git", "-C", w, "add", "-A"], capture=True)
        if run(["git", "-C", w, "diff", "--cached", "--quiet"], check=False).returncode == 0:
            return
        msg = f"e2e: {'up' if files else 'down'} {env}"
        run(["git", "-C", w, "-c", "user.email=ci@local", "-c", "user.name=ci", "commit", "-q", "-m", msg], capture=True)
        run(["git", "-C", w, "push", "-q", "origin", "HEAD:main"], capture=True)
    for appset in ("e2e-infra", "e2e-services"):
        kube("-n", "argocd", "annotate", "applicationset", appset, "argocd.argoproj.io/application-set-refresh=true", "--overwrite", check=False)


def preview_apps(env):
    res = kube("-n", "argocd", "get", "applications", "-l", f"agora/preview={env}", "-o", "json", check=False)
    return json.loads(res.stdout or '{"items": []}')["items"] if res.returncode == 0 else []


def wait_apps(env, want, timeout):
    t0 = time.time()
    while time.time() - t0 < timeout:
        apps = preview_apps(env)
        if want == 0 and not apps:
            return True
        if want and len(apps) >= want:
            bad = [a["metadata"]["name"] for a in apps
                   if a.get("status", {}).get("health", {}).get("status") != "Healthy"
                   or a.get("status", {}).get("sync", {}).get("status") != "Synced"]
            if not bad:
                return True
        time.sleep(10)
    return False


def cmd_preview_up(a):
    env = slug(a.env)
    src, services, owners, run_set = cmd_plan(a)
    chart = {p.stem for p in (GITOPS / "envs" / "services").glob("*.yaml")}
    svcs = sorted((set(run_set) & chart) | ({"team-frontend"} if a.ui else set()))
    skipped = sorted(n for n in run_set if n.startswith(("team-", "platform-")) and not n.endswith("-migrate") and n not in chart)
    infra = sorted({INFRA_FILES[n] for n in run_set if n in INFRA_FILES} | {INFRA_FILES["postgres"], INFRA_FILES["redpanda"]})
    fe, api = preview_hosts(env)
    ensure_cluster()
    built = {base_image(services[o]) for o in owners}
    files = {"env.yaml": {"id": env, "infraInclude": "{" + ",".join(infra) + "}", "services": [{"name": n} for n in svcs]}}
    for n in svcs:
        if n == "team-frontend":
            ref = preview_frontend(src, env, fe, api)
        else:
            local = f"{base_image(services[n])}:{env}"
            ref = local if base_image(services[n]) in built and image_id(local) else f"{base_image(services[n])}:{MAIN_TAG}"
        tag = push_image(ref, n)
        envmap = {k: k8s_value(v, services, fe, api) for k, v in (services[n].get("environment") or {}).items() if v is not None}
        u = n.replace("-", "_")
        for k, v in envmap.items():
            envmap[k] = re.sub(r"postgres://\w+_svc:\w+_pass@postgres:5432/\w+_db", f"postgres://{u}_svc:{u}_pass@postgres:5432/{u}_db", v)
        base = yaml.safe_load((GITOPS / "envs" / "services" / f"{n}.yaml").read_text()) or {}
        for k, v in (base.get("env") or {}).items():
            if k not in envmap and isinstance(v, str) and ".marketplace.svc" in v:
                envmap[k] = k8s_value(v, services, fe, api)
        doc = {"images": {n: tag}, "env": envmap}
        if n == "team-gateway":
            doc["ingress"] = {"enabled": True, "host": api, "path": "/"}
        if n == "team-frontend":
            doc["ingress"] = {"enabled": True, "host": fe, "path": "/", "annotations": None}
        files[f"{n}.yaml"] = doc
        print(f"{n}: {ref} -> {REGISTRY}/{n}:{tag}")
    gitea_write(env, files)
    st_path = state_dir(f"preview-{env}")
    st_path.mkdir(parents=True, exist_ok=True)
    (st_path / "preview.json").write_text(json.dumps({"env": env, "services": svcs, "infra": infra}, indent=2) + "\n")
    if skipped:
        print(f"not deployable by the chart (left out): {' '.join(skipped)}")
    print(f"waiting for {len(svcs) + 1} apps in namespace e2e-{env} ...")
    if not wait_apps(env, len(svcs) + 1, a.timeout):
        sys.exit(f"agora-env: preview e2e-{env} not Healthy after {a.timeout}s (kubectl -n argocd get applications -l agora/preview={env})")
    print(f"preview e2e-{env} Healthy: frontend http://{fe} gateway http://{api}")


def cmd_preview_e2e(a):
    env = slug(a.env)
    fe, api = preview_hosts(env)
    args = list(a.pytest)
    if args and args[0] == "--":
        args = args[1:]
    if not args:
        args = ["-m", "smoke"]
    wt = Path(a.wt or REPO).resolve()
    main_tree = Path(out(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], cwd=REPO).strip()).parent
    py = next((c for c in [r / "platform-e2e" / ".venv" / "bin" / "python" for r in (wt, REPO, main_tree)] if c.exists()), None)
    if not py:
        sys.exit("agora-env: no platform-e2e venv found")
    e = {**os.environ, "HEADLESS": "true", "BASE_URL": f"http://{fe}", "GATEWAY_URL": f"http://{api}"}
    sys.exit(subprocess.run([str(py), "-m", "pytest", "-q", "-p", "no:cacheprovider", *args], cwd=wt / "platform-e2e", env=e).returncode)


def cmd_preview_down(a):
    env = slug(a.env)
    if "marketplace" in out(["kind", "get", "clusters"]).split():
        gitea_write(env, {})
        if not wait_apps(env, 0, a.timeout):
            print(f"apps of e2e-{env} still present after {a.timeout}s")
        kube("delete", "namespace", f"e2e-{env}", "--ignore-not-found", "--wait=false", check=False)
    shutil.rmtree(state_dir(f"preview-{env}"), ignore_errors=True)
    imgs = [r for r in out(["docker", "images", "--format", "{{.Repository}}:{{.Tag}}"]).split() if r.endswith(f":{env}-preview")]
    if imgs:
        run(["docker", "rmi", *imgs], check=False, capture=True)
    print(f"preview e2e-{env} removed")


def cmd_preview_status(a):
    for app in preview_apps(slug(a.env)):
        st = app.get("status", {})
        print(f"{app['metadata']['name']:<40} {st.get('sync', {}).get('status', '?'):<10} {st.get('health', {}).get('status', '?')}")


def main():
    ap = argparse.ArgumentParser(prog="agora_env.py")
    sub = ap.add_subparsers(dest="cmd", required=True)

    def scope(p):
        p.add_argument("--slices")
        p.add_argument("--services", type=lambda s: [x for x in s.split(",") if x])
        p.add_argument("--with", dest="with_", type=lambda s: [x for x in s.split(",") if x])
        p.add_argument("--with-optional", action="store_true")
        p.add_argument("--src")

    p = sub.add_parser("plan")
    scope(p)
    p.set_defaults(fn=cmd_plan_print)
    p = sub.add_parser("up")
    p.add_argument("env")
    scope(p)
    p.add_argument("--branch")
    p.add_argument("--overlay", action="append")
    p.add_argument("--no-build", action="store_true")
    p.set_defaults(fn=cmd_up)
    p = sub.add_parser("e2e-env")
    p.add_argument("env")
    p.set_defaults(fn=cmd_e2e_env)
    p = sub.add_parser("e2e")
    p.add_argument("env")
    p.add_argument("--wt")
    p.add_argument("--all", action="store_true")
    p.add_argument("pytest", nargs=argparse.REMAINDER)
    p.set_defaults(fn=cmd_e2e)
    p = sub.add_parser("dc")
    p.add_argument("env")
    p.add_argument("args", nargs=argparse.REMAINDER)
    p.set_defaults(fn=cmd_dc)
    p = sub.add_parser("down")
    p.add_argument("env")
    p.set_defaults(fn=cmd_down)
    p = sub.add_parser("promote")
    p.add_argument("--ref", default="origin/main")
    p.add_argument("--since")
    p.add_argument("--overlay", action="append")
    p.add_argument("--services", type=lambda s: [x for x in s.split(",") if x])
    p.add_argument("--dry-run", action="store_true")
    p.set_defaults(fn=cmd_promote)
    p = sub.add_parser("gc")
    p.add_argument("--apply", action="store_true")
    p.add_argument("--keep", default="72h")
    p.set_defaults(fn=cmd_gc)
    p = sub.add_parser("preview")
    psub = p.add_subparsers(dest="pcmd", required=True)
    q = psub.add_parser("up")
    q.add_argument("env")
    scope(q)
    q.add_argument("--ui", action="store_true")
    q.add_argument("--timeout", type=int, default=900)
    q.set_defaults(fn=cmd_preview_up)
    q = psub.add_parser("e2e")
    q.add_argument("env")
    q.add_argument("--wt")
    q.add_argument("pytest", nargs=argparse.REMAINDER)
    q.set_defaults(fn=cmd_preview_e2e)
    q = psub.add_parser("down")
    q.add_argument("env")
    q.add_argument("--timeout", type=int, default=300)
    q.set_defaults(fn=cmd_preview_down)
    q = psub.add_parser("status")
    q.add_argument("env")
    q.set_defaults(fn=cmd_preview_status)
    p = sub.add_parser("ls")
    p.add_argument("--porcelain", action="store_true")
    p.set_defaults(fn=cmd_ls)
    a = ap.parse_args()
    a.fn(a)


if __name__ == "__main__":
    main()
