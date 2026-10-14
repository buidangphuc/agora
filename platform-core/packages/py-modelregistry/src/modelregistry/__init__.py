"""Unified model registry client and publish gate (MLflow, ADR-0017)."""
from .client import check_reachable
from .errors import (
    GateError,
    ModelRegistryError,
    PolicyError,
    RegistryUnavailable,
    RollbackError,
)
from .gate import Decision, decide
from .registry import (
    apply,
    evaluate,
    get_champion,
    mark_rejected,
    promote,
    register_rejected,
    rollback,
    seed,
    submit,
)

__all__ = [
    "Decision", "GateError", "ModelRegistryError", "PolicyError", "RegistryUnavailable",
    "RollbackError", "apply", "check_reachable", "decide", "evaluate", "get_champion",
    "mark_rejected", "promote", "register_rejected", "rollback", "seed", "submit",
]
