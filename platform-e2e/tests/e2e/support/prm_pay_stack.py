"""Scratch payment database for the migration scenarios of payment-refund-model (area prm-pay).

A throwaway database on the stack's postgres is created, taken to a given `team-payment`
migration version with the `migrate/migrate` image (the same image and `migrations/` directory the
stack's `team-payment-migrate` service uses), filled through `psql`, and optionally served by a
throwaway `team-payment` container (Kafka off) that is called over gRPC with an admin principal.
The live `payment_db` is never touched. Scenarios that use it are @destructive (serial lane).

Names and locations come from the environment with the compose defaults:

    PRM_PAY_MIGRATIONS_DIR   team-payment/migrations of this checkout
    PRM_PAY_MIGRATE_IMAGE    migrate/migrate:v4.17.1
    PRM_PAY_PG_HOST          hostname of postgres on the stack network (default `postgres`)
    PRM_PAY_PG_USER/PASS     postgres / postgres
    PLP_TEAM_PAYMENT_IMAGE   image of the throwaway team-payment (default agora-team-payment:local)
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from tests.e2e.support import plp_stack as stack

GRPC_PORT = 50056
ADMIN_HEADERS = (
    "x-principal-id: prm-pay-admin",
    "x-principal-type: user",
    "x-principal-scopes: admin",
)
SERVICE = "platform.payment.v1.PaymentService"


def migrations_dir() -> Path:
    env = os.getenv("PRM_PAY_MIGRATIONS_DIR")
    if env:
        return Path(env)
    return Path(__file__).resolve().parents[4] / "team-payment" / "migrations"


def migrate_image() -> str:
    return os.getenv("PRM_PAY_MIGRATE_IMAGE", "migrate/migrate:v4.17.1")


def pg_host() -> str:
    return os.getenv("PRM_PAY_PG_HOST", "postgres")


def _pg_creds() -> str:
    return f"{os.getenv('PRM_PAY_PG_USER', 'postgres')}:{os.getenv('PRM_PAY_PG_PASS', 'postgres')}"


def snake(grpc_code: str) -> str:
    """grpcurl's `FailedPrecondition` -> the Connect code `failed_precondition`."""
    return re.sub(r"(?<!^)(?=[A-Z])", "_", grpc_code).lower()


@dataclass
class ScratchDb:
    name: str = field(default_factory=lambda: f"prm_pay_{uuid.uuid4().hex[:12]}")
    container: str | None = None
    port: int | None = None

    # ── lifecycle ────────────────────────────────────────────────────────
    def create(self) -> None:
        stack.docker(
            "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", "postgres",
            "-v", "ON_ERROR_STOP=1", "-c", f'CREATE DATABASE "{self.name}"',
        )  # fmt: skip

    def drop(self) -> None:
        self.stop_service()
        stack.docker(
            "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", "postgres",
            "-c", f'DROP DATABASE IF EXISTS "{self.name}" WITH (FORCE)',
            check=False,
        )  # fmt: skip

    # ── SQL ──────────────────────────────────────────────────────────────
    def psql(self, sql: str, check: bool = True) -> subprocess.CompletedProcess[str]:
        return stack.docker(
            "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", self.name,
            "-v", "ON_ERROR_STOP=1", "-At", "-F", "|", "-c", sql,
            check=check,
        )  # fmt: skip

    def rows(self, sql: str) -> list[list[str]]:
        proc = self.psql(sql, check=False)
        assert (
            proc.returncode == 0
        ), f"psql on {self.name} failed for {sql!r}: {proc.stderr.strip()[:300]}"
        out = proc.stdout.strip()
        return [line.split("|") for line in out.splitlines() if line]

    def scalar(self, sql: str) -> str:
        rows = self.rows(sql)
        assert rows and rows[0], f"no row for: {sql}"
        return rows[0][0]

    # ── migrations ───────────────────────────────────────────────────────
    def migrate(self, *args: str) -> str:
        """`migrate ... <args>` against the scratch database; the output, or an AssertionError."""
        mdir = migrations_dir()
        assert (
            mdir.is_dir()
        ), f"team-payment migrations not found at {mdir} (PRM_PAY_MIGRATIONS_DIR)"
        proc = stack.docker(
            "run", "--rm", "--network", stack.stack_network(), "-v", f"{mdir}:/migrations:ro",
            migrate_image(), "-path=/migrations",
            f"-database=postgres://{_pg_creds()}@{pg_host()}:5432/{self.name}?sslmode=disable",
            *args,
            check=False, timeout=120,
        )  # fmt: skip
        out = (proc.stdout + proc.stderr).strip()
        assert proc.returncode == 0, f"migrate {' '.join(args)} failed: {out[-600:]}"
        return out

    def version(self) -> str:
        return self.scalar("SELECT version FROM schema_migrations")

    # ── throwaway team-payment ───────────────────────────────────────────
    def start_service(self, timeout_s: float = 40.0) -> None:
        """Run the team-payment image on the scratch database; returns once gRPC answers."""
        name = f"prm-pay-svc-{uuid.uuid4().hex[:8]}"
        env = {
            "DATABASE_ENABLED": "true",
            "DATABASE_URL": f"postgres://{_pg_creds()}@{pg_host()}:5432/{self.name}?sslmode=disable",
            "KAFKA_ENABLED": "false",
            "GRPC_PORT": str(GRPC_PORT),
            "ENV": "local",
            "MOCK_PAYMENTS": "true",
        }
        args = ["run", "-d", "--name", name, "--network", stack.stack_network()]
        args += ["-p", f"127.0.0.1::{GRPC_PORT}"]
        for k, v in env.items():
            args += ["-e", f"{k}={v}"]
        args.append(stack.image_name("team-payment"))
        stack.docker(*args)
        self.container = name
        port_out = stack.docker("port", name, f"{GRPC_PORT}/tcp").stdout.strip().splitlines()[0]
        self.port = int(port_out.rsplit(":", 1)[1])
        deadline = time.monotonic() + timeout_s
        last = ""
        while time.monotonic() < deadline:
            running = stack.docker(
                "inspect", name, "--format", "{{.State.Running}}", check=False
            ).stdout.strip()
            if running != "true":
                logs = stack.docker("logs", name, check=False)
                raise AssertionError(
                    f"throwaway team-payment exited: {(logs.stdout + logs.stderr)[-500:]}"
                )
            proc = self._grpcurl("list", check=False)
            if proc.returncode == 0:
                return
            last = (proc.stdout + proc.stderr).strip()[:200]
            time.sleep(1)
        raise AssertionError(f"throwaway team-payment never answered gRPC: {last}")

    def stop_service(self) -> None:
        if self.container:
            stack.docker("rm", "-f", self.container, check=False)
            self.container = self.port = None

    def _grpcurl(self, *args: str, data: str | None = None, check: bool = True):
        cmd = ["grpcurl", "-plaintext"]
        for h in ADMIN_HEADERS:
            cmd += ["-H", h]
        if data is not None:
            cmd += ["-d", data]
        cmd += [f"127.0.0.1:{self.port}", *args]
        try:
            return subprocess.run(cmd, capture_output=True, text=True, timeout=30, check=check)
        except FileNotFoundError as exc:
            raise AssertionError(
                "grpcurl is not installed on this machine (brew install grpcurl)"
            ) from exc

    def grpc_refund(
        self, payment_id: str, amount: int, refund_id: str, reason: str = "e2e"
    ) -> tuple[str, dict | str]:
        """RefundPayment as an admin on the throwaway service: (connect-style code, body or error)."""
        body = json.dumps(
            {"paymentId": payment_id, "amount": amount, "reason": reason, "refundId": refund_id}
        )
        proc = self._grpcurl(f"{SERVICE}/RefundPayment", data=body, check=False)
        if proc.returncode == 0:
            return "ok", json.loads(proc.stdout or "{}")
        err = proc.stderr + proc.stdout
        m = re.search(r"Code:\s*(\w+)", err)
        return (snake(m.group(1)) if m else f"grpcurl_exit_{proc.returncode}"), err.strip()
