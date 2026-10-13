"""Redis keys written by platform-recsys and platform-featurestore, read here.

Every builder mirrors its producer (``platform-recsys/recsys/config.py``,
``recsys/nearline/signals.py``, ``platform-featurestore/featurestore/online.py``). A change
here is a break with the producers: ``tests/unit/modules/recommendation/test_contract.py``
pins every string.
"""

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class ServingKeys:
    """The serving cache of platform-recsys, on the serving Redis.

    ``<prefix>:<schema>:serving`` names the generation being served. Its keys are
    ``<prefix>:<schema>:gen:<gen>:...``; with no generation the legacy unscoped keys apply.
    """

    prefix: str
    schema_version: str

    @property
    def _base(self) -> str:
        return f"{self.prefix}:{self.schema_version}"

    def _scope(self, generation: str | None) -> str:
        return f"{self._base}:gen:{generation}" if generation else self._base

    @property
    def serving(self) -> str:
        return f"{self._base}:serving"

    @property
    def model_version(self) -> str:
        return f"{self._base}:model_version"

    def user(self, user_id: str, generation: str | None) -> str:
        return f"{self._scope(generation)}:user:{user_id}"

    def popular(self, generation: str | None) -> str:
        return f"{self._scope(generation)}:popular"

    def ranker(self, generation: str) -> str:
        """The trained GBDT ranker artifact of ``generation`` (JSON ``agora-gbdt/1``)."""
        return f"{self._scope(generation)}:ranker"


def featurestore_current_key(view: str) -> str:
    """The version of ``view`` currently served, on the featurestore Redis."""
    return f"fs:{view}:current"


def featurestore_row_key(view: str, version: str, entity_id: str) -> str:
    return f"fs:{view}:v{version}:{entity_id}"


def nearline_ctr_key(prefix: str, listing_id: str) -> str:
    """HASH with float fields ``clicks_ips`` and ``imprs_ips``, on the nearline Redis."""
    return f"{prefix}:ctr:{listing_id}"
