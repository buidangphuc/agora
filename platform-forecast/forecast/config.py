"""Configuration for platform-forecast batch pipeline."""

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    redis_url: str = "redis://localhost:6379/0"
    input_dir: str = "/data/input"
    as_of: str = ""
    backtest_days: int = 14

    horizon_days: int = 14
    history_days: int = 90
    min_history_days_for_ml: int = 14

    redis_prefix: str = "fc:v1:seller"
    redis_ttl_seconds: int = 172800

    @property
    def quantiles(self) -> list[float]:
        return [0.10, 0.50, 0.90]

    @property
    def model_version(self) -> str:
        return "lgbm_quantile_v1"
