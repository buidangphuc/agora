"""The listing.events consumer must read what the Go producers write.

franz-go compresses record batches with snappy by default; aiokafka only decodes
it when ``cramjam`` is installed (the ``kafka`` extra). Without it the consumer
dies with ``UnsupportedCodecError`` on the first fetch.
"""

from __future__ import annotations

import pytest

pytest.importorskip("aiokafka")

from aiokafka.record.default_records import (
    DefaultRecordBatch,
    DefaultRecordBatchBuilder,
)


@pytest.mark.parametrize("compression", ["snappy", "lz4", "zstd"])
def test_compressed_record_batch_decodes(compression: str) -> None:
    codec = {"snappy": 2, "lz4": 3, "zstd": 4}[compression]
    builder = DefaultRecordBatchBuilder(
        magic=2,
        compression_type=codec,
        is_transactional=0,
        producer_id=-1,
        producer_epoch=-1,
        base_sequence=-1,
        batch_size=1 << 16,
    )
    for offset in range(3):
        builder.append(offset, 1_700_000_000_000, b"k", b"listing-%d" % offset, [])
    raw = bytes(builder.build())

    batch = DefaultRecordBatch(raw)
    assert [bytes(r.value) for r in batch] == [b"listing-0", b"listing-1", b"listing-2"]
