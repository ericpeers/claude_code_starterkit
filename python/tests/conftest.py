# SPDX-License-Identifier: MIT
"""Suite-wide pytest hooks."""
from __future__ import annotations

import os
import tempfile


def pytest_configure(config) -> None:
    """Isolates the run from the git hook environment before any test starts."""
    isolate_git()


def git_env_vars() -> list[str]:
    """The GIT_* variables in the environment, sorted."""
    return sorted(name for name in os.environ if name.startswith("GIT_"))


def isolate_git() -> None:
    """Deletes every GIT_* variable and stops the git repo search at the temp dir."""
    for name in git_env_vars():
        del os.environ[name]
    os.environ["GIT_CEILING_DIRECTORIES"] = tempfile.gettempdir()
