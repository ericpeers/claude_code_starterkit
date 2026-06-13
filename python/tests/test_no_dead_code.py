# SPDX-License-Identifier: MIT
"""
Dead-code / unused-symbol gate, run as part of the normal pytest suite.

Two layers, mirroring industry practice:

* **ruff --select F** (pyflakes) — unused imports, redefinitions, unused
  locals, duplicate dict keys, undefined names. Near-zero false positives, so
  this is a hard gate.

* **vulture --min-confidence 60** — unused *functions/classes/methods*, which
  pyflakes does not catch. Confidence 60 (not 80) is deliberate: vulture only
  assigns >=80 to unused imports/variables, so 80+ is blind to dead functions
  and would just duplicate ruff. Intentional-but-uncalled symbols (public API,
  test-only helpers) are listed in ``vulture_whitelist.py``.

Both tools are skipped (not failed) if not installed, so the suite still runs
in a bare environment. Install them with ``pip install ruff vulture``.

Customize SCAN_PATHS to point at your production source (exclude tests).
"""
from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent

# Production code scanned for dead code. Tests are excluded as a scan target:
# test files legitimately define fixtures/helpers that look unused, and symbols
# used *only* by tests are whitelisted instead (see vulture_whitelist.py).
# EDIT THIS LIST to match your package layout. Entries that don't exist yet are
# filtered out so a fresh scaffold still passes.
SCAN_PATHS = [
    "src",
    "app",
    "config.py",
]


def _existing(paths: list[str]) -> list[str]:
    """Keep only scan targets that currently exist on disk."""
    return [p for p in paths if (ROOT / p).exists()]


def _run(module: str, args: list[str]) -> subprocess.CompletedProcess[str]:
    """Run ``python -m <module> <args>`` from the repo root, or skip if absent."""
    try:
        return subprocess.run(
            [sys.executable, "-m", module, *args],
            cwd=ROOT,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:  # pragma: no cover - environment without the tool
        pytest.skip(f"{module} not installed")


def test_no_pyflakes_dead_code() -> None:
    """ruff pyflakes rules (F) must be clean across production code."""
    targets = _existing(SCAN_PATHS)
    if not targets:
        pytest.skip("no SCAN_PATHS exist yet")
    proc = _run("ruff", ["check", "--select", "F", "--no-cache", *targets])
    if proc.returncode == 2:  # ruff itself errored (e.g. not installed)
        pytest.skip(f"ruff unavailable: {proc.stderr.strip()}")
    assert proc.returncode == 0, (
        "ruff found unused imports / dead code (F rules):\n"
        f"{proc.stdout}\n{proc.stderr}"
    )


def test_no_unused_functions() -> None:
    """vulture must find no unused functions/classes beyond the whitelist."""
    targets = _existing(SCAN_PATHS)
    if not targets:
        pytest.skip("no SCAN_PATHS exist yet")
    proc = _run(
        "vulture",
        [*targets, "vulture_whitelist.py", "--min-confidence", "60"],
    )
    # vulture exit codes: 0 = clean, 3 = dead code found, others = tool error.
    if proc.returncode not in (0, 3):  # pragma: no cover
        pytest.skip(f"vulture unavailable: {proc.stderr.strip()}")
    assert proc.returncode == 0, (
        "vulture found unused code. Either remove it, or — if it is intentional "
        "(public API, used only by tests, dynamic dispatch) — add it to "
        "vulture_whitelist.py:\n"
        f"{proc.stdout}\n{proc.stderr}"
    )
