"""Enable running the E2E suite via ``python e2e`` or ``python -m e2e``."""

from __future__ import annotations

import pathlib
import sys


def _load_main():
    try:
        # Preferred path when executed as a module (``python -m e2e``).
        from .main import main as run_main  # type: ignore
    except ImportError:
        # Fallback for ``python e2e`` where the directory is executed as a script
        # and relative imports are not resolved automatically.
        root = pathlib.Path(__file__).resolve().parent.parent
        if str(root) not in sys.path:
            sys.path.insert(0, str(root))
        from e2e.main import main as run_main  # type: ignore
    return run_main


if __name__ == "__main__":
    raise SystemExit(_load_main()())
