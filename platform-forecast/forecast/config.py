"""Configuration for platform-forecast batch pipeline."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    env: str = "local"
    redis_url: str = "redis://localhost:6379/0"
    duckdb_path: str = "/data/analytics.duckdb"
    
    # Model & forecasting params
    model_version: str = "lgbm_quantile_v1"
    horizon_days: int = 28
    history_days: int = 90
    min_history_days_for_ml: int = 14
    
    # Quantiles
    quantiles: list[float] = [0.10, 0.50, 0.90]
    
    # Redis export
    redis_prefix: str = "fc:v1:seller"
    redis_ttl_seconds: int = 172800  # 48 hours

    class Config:
        env_file = ".env"
        extra = "ignore"
