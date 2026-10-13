"""Subprocess driver for test_cli_exit_codes.py: runs `python main.py run` with the heavy stages stubbed.

Not a test module. Configured by env:
  FT_TEST_ROOT     tmp artifact root
  FT_TEST_MODE     "ok" (train stub writes model + stats) | "raise" (train stub raises)
  FT_TEST_RUN_ID   MLflow run id the stubbed trainer "created"
  FT_TEST_METRICS  JSON metrics the stubbed evaluation produces
Stage starts are appended to <root>/stages.log. Ingest/validate/transform/train are stubs; `evaluate`
is the REAL `stages.run_evaluate` (only the heavy `ModelEvaluation` component is faked).
INTERFACE ASSUMPTIONS: the trainer hands its run id to evaluate through `run_id` in
paths.TRAIN_STATS; `stages._STAGES` is the ordered stage list used by `run_all`.
"""

import json
import os
import runpy
import sys
from pathlib import Path

root = Path(os.environ["FT_TEST_ROOT"])
mode = os.environ.get("FT_TEST_MODE", "ok")

from datascience.constants import paths  # noqa: E402

mapping = {
    "ARTIFACTS": root,
    "EVAL_DIR": root / "model_evaluation",
    "METRICS_FILE": root / "model_evaluation" / "metrics.json",
    "SPLIT_DIR": root / "data_transformation",
    "DATA_VERSION": root / "data_transformation" / "data_version.json",
    "TRAINER_DIR": root / "model_trainer",
    "MODEL_DIR": root / "model_trainer" / "model",
    "CKPT_DIR": root / "model_trainer" / "checkpoints",
    "TRAIN_STATS": root / "model_trainer" / "train_stats.json",
    "RUN_STATUS": root / "run_status.json",
    "LOG_DIR": root / "logs",
}
for _name, _value in mapping.items():
    setattr(paths, _name, _value)
paths.ensure_dirs = lambda: None

from datascience.pipeline import stages  # noqa: E402

_log = root / "stages.log"
_real_evaluate = stages.run_evaluate


def _mark(name):
    with _log.open("a") as fh:
        fh.write(name + "\n")


def ingest():
    _mark("ingest")


def validate():
    _mark("validate")


def transform():
    _mark("transform")
    paths.DATA_VERSION.parent.mkdir(parents=True, exist_ok=True)
    paths.DATA_VERSION.write_text(json.dumps({"sha256": "abc123def456", "n_rows": 10}))


def train():
    _mark("train")
    if mode == "raise":
        raise RuntimeError("boom from the stubbed train stage")
    paths.MODEL_DIR.mkdir(parents=True, exist_ok=True)
    (paths.MODEL_DIR / "model.safetensors").write_text("w")
    paths.TRAIN_STATS.write_text(json.dumps({"run_id": os.environ["FT_TEST_RUN_ID"], "samples_per_s": 1.0}))


class _FakeEval:
    def __init__(self, cfg):
        pass

    def evaluate(self):
        paths.METRICS_FILE.parent.mkdir(parents=True, exist_ok=True)
        paths.METRICS_FILE.write_text(os.environ["FT_TEST_METRICS"])


def evaluate():
    _mark("evaluate")
    _real_evaluate()


stages.ModelEvaluation = _FakeEval
stages._STAGES = [("ingest", ingest), ("validate", validate), ("transform", transform),
                  ("train", train), ("evaluate", evaluate)]

sys.argv = ["main.py", "run"]
runpy.run_path(str(Path(__file__).resolve().parent.parent / "main.py"), run_name="__main__")
