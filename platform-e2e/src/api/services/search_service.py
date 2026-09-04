"""Search service: query the OpenSearch-backed read-model via the gateway."""

from __future__ import annotations

from src.constants import gateway_endpoints as ep

from .base_service import BaseService


class SearchService(BaseService):
    def search(self, query: str) -> list[dict]:
        data = self.post(ep.SEARCH_LISTINGS, {"query": query})
        result = data.get("result") or data
        return result.get("listings", [])
