"""Exception types. CLI exit: RegistryUnavailable -> 1, other ModelRegistryError -> 2."""
from __future__ import annotations


class ModelRegistryError(Exception):
    """Base class for library errors."""


class PolicyError(ModelRegistryError):
    """Unknown model or malformed policy entry."""


class GateError(ModelRegistryError):
    """Operation refused by the gate state model (wrong state, unknown version, used model)."""


class RollbackError(ModelRegistryError):
    """Rollback is impossible (no previous version)."""


class RegistryUnavailable(ModelRegistryError):
    """The registry did not answer in time or at all. The gate fails closed."""
