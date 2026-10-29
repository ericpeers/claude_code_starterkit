# SPDX-License-Identifier: MIT
"""
Gate that the suite runs only read-only git.

CLAUDE.md, "Read-only git in tests", gives the rule and the reason.
"""
from __future__ import annotations

import ast
import re
from pathlib import Path

import pytest

from tests.conftest import git_env_vars, isolate_git

_TEST_DIR = Path(__file__).parent

# the git subcommands a test may run
_READ_ONLY = {"ls-files", "log", "check-ignore"}

# global options whose value is the next argument
_TAKES_VALUE = {"-C", "--git-dir", "--work-tree", "--namespace"}

# calls that run a shell command line
_SHELL_RUNNERS = {
    "run", "call", "check_call", "check_output", "Popen", "getoutput",
    "getstatusoutput", "system", "popen", "create_subprocess_shell",
}

# libraries that write to a repo without the git binary
_GIT_LIBRARIES = {"git", "pygit2", "dulwich"}

# git in a shell command line, then the rest of its command up to a separator
_SHELL_GIT = re.compile(r"(?<![\w./-])git(?![\w-])([^;&|\n]*)")

# the word git anywhere in a shell command line
_GIT_WORD = re.compile(r"(?<![\w./-])git(?![\w-])")


def _argv_reason(args: list[ast.expr]) -> str:
    """Why the arguments after git can write, or "" when they only read."""
    i = 0
    while i < len(args):
        node = args[i]
        if not (isinstance(node, ast.Constant) and isinstance(node.value, str)):
            return "the git subcommand is not a string literal"
        arg = node.value
        if arg == "-c" or arg.startswith("--config-env"):
            return f"git {arg} can set an alias or a hook"
        if arg in _TAKES_VALUE:
            i += 2
            continue
        if arg.startswith("-"):
            i += 1
            continue
        return "" if arg in _READ_ONLY else f"git {arg} is not a read-only subcommand"
    return "the git call names no subcommand"


def _words_reason(words: list[str]) -> str:
    """Why the words after git in a shell command can write, or "" when they only read."""
    i = 0
    while i < len(words):
        word = words[i]
        if word == "-c" or word.startswith("--config-env"):
            return f"git {word} can set an alias or a hook"
        if word in _TAKES_VALUE:
            i += 2
            continue
        if word.startswith("-"):
            i += 1
            continue
        return "" if word in _READ_ONLY else f"git {word} is not a read-only subcommand"
    return ""


def _shell_reason(command: str) -> str:
    """Why a shell command line runs a git write, or "" when it runs none."""
    for match in _SHELL_GIT.finditer(command):
        if reason := _words_reason(match.group(1).split()):
            return reason
    return ""


def _shell_arg_reason(node: ast.expr) -> str:
    """Why a shell runner's first argument can run a git write, or "" when it runs none."""
    if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute) and node.func.attr == "split":
        node = node.func.value
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return _shell_reason(node.value)
    if isinstance(node, ast.JoinedStr) and any(
        isinstance(part, ast.Constant) and isinstance(part.value, str) and _GIT_WORD.search(part.value)
        for part in node.values
    ):
        return "the git command line is not a string literal"
    return ""


def _is_git(node: ast.expr) -> bool:
    """Whether a node is the string literal "git"."""
    return isinstance(node, ast.Constant) and node.value == "git"


def _callee_name(call: ast.Call) -> str:
    """The bare name of the function a call invokes."""
    func = call.func
    if isinstance(func, ast.Attribute):
        return func.attr
    return func.id if isinstance(func, ast.Name) else ""


def _node_reason(node: ast.AST) -> str:
    """Why one syntax node can write to a git repo, or "" when it runs none."""
    if isinstance(node, (ast.List, ast.Tuple)) and node.elts and _is_git(node.elts[0]):
        return _argv_reason(node.elts[1:])
    if isinstance(node, ast.Import):
        names = {alias.name.split(".")[0] for alias in node.names}
        if names & _GIT_LIBRARIES:
            return f"imports {', '.join(sorted(names & _GIT_LIBRARIES))}"
    if isinstance(node, ast.ImportFrom) and (node.module or "").split(".")[0] in _GIT_LIBRARIES:
        return f"imports {node.module}"
    if not isinstance(node, ast.Call):
        return ""
    name = _callee_name(node)
    if name == "git":
        return _argv_reason(node.args)
    if node.args and _is_git(node.args[0]):
        return _argv_reason(node.args[1:])
    if name in _SHELL_RUNNERS and node.args:
        return _shell_arg_reason(node.args[0])
    return ""


def _git_writes(source: str, path: str) -> list[str]:
    """A "path:line: reason" entry for each git write a source file names."""
    return [
        f"{path}:{getattr(node, 'lineno', 0)}: {reason}"
        for node in ast.walk(ast.parse(source))
        if (reason := _node_reason(node))
    ]


@pytest.mark.parametrize(
    "source, flagged",
    [
        ('subprocess.run(["git", "ls-files", "-z"])', False),
        ('subprocess.run(["git", "-C", repo, "log", "-1"])', False),
        ('subprocess.check_output(("git", "check-ignore", "--stdin"))', False),
        ('subprocess.run(["python", "commit"])', False),
        ('subprocess.run(["git", "commit", "-m", "x"])', True),
        ('subprocess.run(["git", "init"])', True),
        ('subprocess.run(["git", "status"])', True),
        ('cmd = ["git", "config", "user.name", "x"]', True),
        ('subprocess.run(["git", "-c", "core.hooksPath=x", "log"])', True),
        ('subprocess.run(["git", "--config-env=core.hooksPath=X", "log"])', True),
        ('subprocess.run(["git", *args])', True),
        ('subprocess.run(["git", sub, "x"])', True),
        ('subprocess.run(["git"])', True),
        ('pytester.run("git", "init")', True),
        ('asyncio.create_subprocess_exec("git", "add", ".")', True),
        ('git("commit", "-q")', True),
        ('subprocess.run("git -C repo commit -qm x", shell=True)', True),
        ('os.system("cd x && git init -q")', True),
        ('subprocess.run("git -C repo log -1", shell=True)', False),
        ('subprocess.run("echo digit commit", shell=True)', False),
        ('subprocess.run("command -v git", shell=True)', False),
        ('subprocess.run(f"git {sub}", shell=True)', True),
        ('subprocess.run(f"echo {x}", shell=True)', False),
        ('subprocess.run("git commit -m x".split())', True),
        ('subprocess.run("git log -1".split())', False),
        ('subprocess.run(["git", "--git-dir", d, "commit"])', True),
        ('subprocess.run(["git", "--git-dir", d, "log"])', False),
        ('subprocess.run("git --no-pager log -1 foo", shell=True)', False),
        ('subprocess.run("git -C repo -c core.hooksPath=x log", shell=True)', True),
        ("import git", True),
        ("from pygit2 import Repository", True),
    ],
)
def test_the_scan_flags_git_writes(source, flagged):
    """The scan flags each write, option or library it guards against, and no read."""
    assert bool(_git_writes(source, "fixture.py")) is flagged


def test_no_test_file_runs_a_git_write():
    """No file under tests/ runs a git write."""
    violations = [
        entry
        for path in sorted(_TEST_DIR.rglob("*.py"))
        for entry in _git_writes(path.read_text(), str(path.relative_to(_TEST_DIR.parent)))
    ]
    assert not violations, "tests run only read-only git. Stub git instead:\n" + "\n".join(violations)


def test_no_git_hook_variable_reaches_the_tests():
    """The run starts with GIT_CEILING_DIRECTORIES as its only GIT_* variable."""
    assert git_env_vars() == ["GIT_CEILING_DIRECTORIES"]


def test_isolate_git_removes_hook_variables(monkeypatch):
    """A GIT_DIR from the hook environment is gone after isolation."""
    monkeypatch.setenv("GIT_DIR", "/tmp/hook-repo/.git")
    monkeypatch.setenv("GIT_INDEX_FILE", "/tmp/hook-repo/.git/index")
    isolate_git()
    assert git_env_vars() == ["GIT_CEILING_DIRECTORIES"]
