# Ops

Serving and Triton build files live here (`serve-cpu/`, `triton/`). They are NOT PORTED to the
platform yet (see the repo README).

The local MinIO/MLflow compose stack was dropped. Tracking and the model registry use the platform
MLflow server: set `MLFLOW_TRACKING_URI` (the fixed sqlite default in `constants/paths.py` is used
only when it is unset).
