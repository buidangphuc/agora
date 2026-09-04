# migrations/

Alembic migration scripts for **this service's own** database live here.
Placeholder — no migrations exist yet in the seed.

Rule 3: a service owns its schema. Never write a migration that touches another
service's tables; cross-service data comes over gRPC.

## Activating alembic

1. Add `alembic` and an async driver (e.g. `asyncpg`) to `pyproject.toml`.
2. Scaffold the environment:
   ```bash
   uv run alembic init migrations
   ```
   Then edit `migrations/env.py` to pull the URL from settings instead of
   `alembic.ini`:
   ```python
   from config import get_settings
   config.set_main_option("sqlalchemy.url", get_settings().DATABASE_URL)
   ```
3. Create and apply migrations:
   ```bash
   uv run alembic revision --autogenerate -m "init"
   uv run alembic upgrade head
   ```
4. Uncomment the `migrate` target in the `Makefile`.
