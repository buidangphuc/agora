"""Process entrypoint: `python -m main` or the `team-service` console script."""

from __future__ import annotations

import asyncio

from server import serve


def main() -> None:
    asyncio.run(serve())


if __name__ == "__main__":
    main()
