# SPDX-License-Identifier: MIT
"""
Dependency vulnerability gate, run as part of the normal pytest suite.

It scans the installed packages with pip-audit, which needs network access to
PyPI. Fix a failure with `python scripts/fix_vulns.py`.
"""
from __future__ import annotations

import importlib.util
import json
import subprocess
from functools import partial

import pytest

from scripts import fix_vulns
from scripts.fix_vulns import Finding, classify, load_allowlist, scan


def _finding(fix: tuple[str, ...] = (), aliases: tuple[str, ...] = ()) -> Finding:
    """A finding against a placeholder package, with the given fixes and aliases."""
    return Finding("pkg", "1.0", "PYSEC-0000-1", aliases, fix, "summary")


def test_no_known_vulnerabilities() -> None:
    """Installed packages carry no fixable or untriaged advisory."""
    if importlib.util.find_spec("pip_audit") is None:
        pytest.fail("pip-audit not installed. Run ./setup_dev.sh or pip install -r requirements.txt.")
    upgrade, now_fixed, triage = classify(scan(), load_allowlist())
    sections = []
    if upgrade:
        sections.append("Vulnerable packages with fixes available:\n"
                        + "\n".join(sorted(finding.line() for finding in upgrade)))
    if now_fixed:
        sections.append("Allowlisted vulnerabilities now have fixes:\n"
                        + "\n".join(sorted(finding.line() for finding in now_fixed)))
    if triage:
        sections.append("Unfixed vulnerabilities not in the allowlist:\n"
                        + "\n".join(sorted(finding.line() for finding in triage)))
    assert not sections, (
        "pip-audit failed. Run `python scripts/fix_vulns.py`.\n\n" + "\n\n".join(sections)
    )


def test_classify_policy() -> None:
    """Fixable fails, unfixed fails until allowlisted, allowlisted fails once fixed."""
    fixable, unfixed = _finding(fix=("2.0",)), _finding()
    assert classify([fixable, unfixed], {}) == ([fixable], [], [unfixed])
    assert classify([unfixed], {"PYSEC-0000-1": "r"}) == ([], [], [])
    assert classify([fixable], {"PYSEC-0000-1": "r"}) == ([], [fixable], [])


def test_allowlist_matches_alias() -> None:
    """An allowlist entry keyed by CVE matches an advisory filed under a PYSEC id."""
    finding = _finding(aliases=("CVE-2026-1", "GHSA-xxxx"))
    assert classify([finding], {"CVE-2026-1": "r"}) == ([], [], [])


def test_allowlist_entries_have_reasons() -> None:
    """Every allowlist entry states why it is accepted."""
    missing = [advisory_id for advisory_id, reason in load_allowlist().items() if not reason.strip()]
    assert not missing, f"Allowlist entries without a reason: {missing}"


def test_allowlist_reason_check_detects_a_blank_reason(tmp_path, monkeypatch) -> None:
    """An entry with a blank reason loads as blank, so the reason check flags it."""
    allowlist = tmp_path / "vuln_allowlist.json"
    allowlist.write_text(json.dumps({"allowlist": [{"id": "PYSEC-1", "reason": " "}, {"id": "PYSEC-2"}]}))
    monkeypatch.setattr(fix_vulns, "ALLOWLIST_PATH", allowlist)
    with pytest.raises(AssertionError, match="PYSEC-1.*PYSEC-2"):
        test_allowlist_entries_have_reasons()


def test_max_fix_picks_highest_version() -> None:
    """The upgrade target is the highest fix, compared as versions."""
    assert _finding(fix=("9.1", "10.0", "9.10")).max_fix == "10.0"


def test_raise_requirement_floor(tmp_path, monkeypatch) -> None:
    """A lower floor rises. A higher floor, comments and other packages stay."""
    reqs = tmp_path / "requirements.txt"
    reqs.write_text(
        "My_Pkg[extra]>=1.0     # why it is here\n"
        "other>=1.0\n"
        "fixed==1.0\n"
    )
    monkeypatch.setattr(fix_vulns, "REQUIREMENTS_PATH", reqs)

    assert fix_vulns._raise_requirement_floor("my-pkg", "1.5")
    assert not fix_vulns._raise_requirement_floor("my-pkg", "1.2")
    assert not fix_vulns._raise_requirement_floor("fixed", "2.0")
    assert reqs.read_text() == (
        "My_Pkg[extra]>=1.5     # why it is here\n"
        "other>=1.0\n"
        "fixed==1.0\n"
    )


def _fake_audit(stdout: str, args, **kwargs) -> subprocess.CompletedProcess:
    """A stand-in for the pip-audit subprocess that prints the given report."""
    return subprocess.CompletedProcess(args, 1, stdout=stdout, stderr="network down")


def test_scan_parses_report_and_drops_duplicates(monkeypatch) -> None:
    """Each (package, id) pair appears once, with its aliases and fixes."""
    vuln = {"id": "PYSEC-1", "aliases": ["CVE-1"], "fix_versions": ["2.0"], "description": "d"}
    report = {"dependencies": [
        {"name": "pkg", "version": "1.0", "vulns": [vuln, dict(vuln, description="dup")]},
        {"name": "clean", "version": "3.0", "vulns": []},
    ]}
    monkeypatch.setattr(fix_vulns.subprocess, "run", partial(_fake_audit, json.dumps(report)))

    assert scan() == [Finding("pkg", "1.0", "PYSEC-1", ("CVE-1",), ("2.0",), "d")]


def test_scan_raises_on_empty_output(monkeypatch) -> None:
    """No report on stdout raises with the pip-audit stderr."""
    monkeypatch.setattr(fix_vulns.subprocess, "run", partial(_fake_audit, ""))
    with pytest.raises(RuntimeError, match="network down"):
        scan()


def test_gate_detects_a_fixable_finding(monkeypatch) -> None:
    """A scan that reports a fixable advisory fails the gate and names the package."""
    vuln = {"id": "PYSEC-1", "aliases": [], "fix_versions": ["2.0"], "description": "d"}
    report = {"dependencies": [{"name": "pkg", "version": "1.0", "vulns": [vuln]}]}
    monkeypatch.setattr(fix_vulns.subprocess, "run", partial(_fake_audit, json.dumps(report)))
    monkeypatch.setattr(importlib.util, "find_spec", _any_spec)
    with pytest.raises(AssertionError, match="pkg 1.0: PYSEC-1 fixed in 2.0"):
        test_no_known_vulnerabilities()


def _any_spec(name: str) -> object:
    """A stand-in for find_spec that reports every module as installed."""
    return object()
