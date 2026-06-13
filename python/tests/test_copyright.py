# SPDX-License-Identifier: MIT
"""
License / copyright header gate for Python sources.

Two kinds of files coexist in a project built from this kit:
  * Kit-supplied boilerplate declares ``SPDX-License-Identifier: MIT`` and is
    EXEMPT here. It carries no personal copyright holder by design.
  * Your own source must carry a current-year copyright header naming the
    project's configured holder (the ``.copyright-holder`` file written by
    setup_dev.sh). This is how you enforce your ownership on the code you write.

A fresh scaffold is green (everything shipped is SPDX-exempt). The gate starts
enforcing once you add your own .py files without an SPDX tag or copyright header.
"""
from __future__ import annotations

import os
import re
import subprocess
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CURRENT_YEAR = date.today().year
YEAR = str(CURRENT_YEAR)
SPDX_RE = re.compile(r"SPDX-License-Identifier:\s*\S+")
EXCLUDED = {".git", "node_modules", ".venv", "venv", "__pycache__", "dist", "build"}


def _holder() -> str | None:
    f = ROOT / ".copyright-holder"
    if f.exists():
        h = f.read_text(encoding="utf-8").strip()
        if h:
            return h
    return os.environ.get("COPYRIGHT_HOLDER", "").strip() or None


def _py_files() -> list[Path]:
    return [p for p in ROOT.rglob("*.py") if not (EXCLUDED & set(p.parts))]


def _head(p: Path) -> str:
    return "\n".join(p.read_text(encoding="utf-8").splitlines()[:5])


def _git_last_commit_year(rel: str) -> int:
    """Year of the most recent commit that touched ``rel`` (relative to ROOT), or
    0 if git is unavailable or the file has never been committed. Callers treat 0
    as the current year, so new/uncommitted files must carry the current year."""
    try:
        proc = subprocess.run(
            ["git", "log", "--follow", "-1", "--format=%ad", "--date=format:%Y", "--", rel],
            cwd=ROOT,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:
        return 0
    text = proc.stdout.strip()
    return int(text) if text.isdigit() else 0


def test_license_headers() -> None:
    holder = _holder()
    non_exempt = [p for p in _py_files() if not SPDX_RE.search(_head(p))]

    if holder is None:
        rels = "\n".join(str(p.relative_to(ROOT)) for p in non_exempt)
        assert not non_exempt, (
            "copyright holder not set. Create .copyright-holder (or run ./setup_dev.sh), then add\n"
            f"  # Copyright (c) {YEAR} <holder>\n"
            "to these files (or give them an SPDX identifier):\n" + rels
        )
        return

    # Presence only: any year or year-range followed by the holder. The year's
    # currency is checked separately against the file's last git-commit year, so
    # a dormant prior-year file isn't forced to bump on every Jan 1.
    presence_re = re.compile(rf"Copyright \(c\) \d{{4}}(?:-\d{{4}})?\s+{re.escape(holder)}")

    violations: list[str] = []
    for p in non_exempt:
        head = _head(p)
        rel = str(p.relative_to(ROOT))
        if not presence_re.search(head):
            violations.append(f"{rel}: missing a `Copyright (c) <year> {holder}` header or SPDX tag")
            continue
        # Require the current year only when the file was last committed this year,
        # or when its commit year is unknown (new/uncommitted/git unavailable).
        last_year = _git_last_commit_year(rel)
        if (last_year == 0 or last_year == CURRENT_YEAR) and YEAR not in head:
            violations.append(f"{rel}: copyright header must include {YEAR} (file changed this year)")

    assert not violations, "copyright violations:\n" + "\n".join(violations)
