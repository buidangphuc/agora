"""Root conftest: ensure the project root is importable as the top-level package root.

Lets `import src...`, `import config...`, and `import tests...` resolve regardless
of pytest's import mode or where it is invoked from.
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
