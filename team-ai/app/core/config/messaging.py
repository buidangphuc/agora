"""Messaging settings: the listing.events Kafka consumer."""

from __future__ import annotations

from pydantic import BaseModel, Field, model_validator


class MessagingSettingsMixin(BaseModel):
    # Declared by AISettingsMixin; annotated here so the rule below can read it.
    RAG_ENABLED: bool

    # listing.events -> RAG indexer (Kafka consumer; runs with RAG_ENABLED and needs
    # the `kafka` extra)
    KAFKA_BROKERS: str = "localhost:9092"  # comma-separated bootstrap servers
    LISTING_INDEXER_TOPIC: str = Field(default="listing.events", min_length=1)
    LISTING_INDEXER_GROUP: str = Field(default="team-ai-indexer", min_length=1)
    LISTING_INDEXER_MAX_ATTEMPTS: int = Field(default=5, gt=0)
    LISTING_INDEXER_BASE_BACKOFF_SECONDS: float = Field(default=0.1, ge=0)

    @model_validator(mode="after")
    def _rag_needs_kafka_brokers(self) -> MessagingSettingsMixin:
        if self.RAG_ENABLED and not self.KAFKA_BROKERS:
            raise ValueError("RAG_ENABLED requires KAFKA_BROKERS")
        return self
