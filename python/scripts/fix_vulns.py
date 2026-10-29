# SPDX-License-Identifier: MIT
"""
Upgrades vulnerable packages in the virtualenv, then triages the rest.

Usage: python scripts/fix_vulns.py. The policy and the allowlist format are in
the dependency vulnerability gate section of CLAUDE_python.md.
"""
from __future__ import annotations

import json
import re
import subprocess
import sys
from dataclasses import dataclass
from importlib.metadata import distributions
from pathlib import Path

from packaging.requirements import InvalidRequirement, Requirement
from packaging.version import InvalidVersion, Version

ROOT = Path(__file__).resolve().parent.parent
ALLOWLIST_PATH = ROOT / "tests" / "vuln_allowlist.json"
REQUIREMENTS_PATH = ROOT / "requirements.txt"


@dataclass(frozen=True)
class Finding:
    """One advisory against one installed package."""

    package: str
    version: str
    id: str
    aliases: tuple[str, ...]
    fix_versions: tuple[str, ...]
    description: str

    @property
    def max_fix(self) -> str:
        """The highest released fix version, or "" when none exists."""
        return max(self.fix_versions, key=_version_key, default="")

    def line(self) -> str:
        """A one-line report of the finding."""
        cve = next((alias for alias in self.aliases if alias.startswith("CVE-")), "")
        label = f"{self.id} ({cve})" if cve else self.id
        fix = f" fixed in {self.max_fix}" if self.fix_versions else ""
        summary = re.sub(r"^(Impact|Summary|Description) ", "",
                         " ".join(self.description.replace("#", "").split()))[:90]
        return f"  {self.package} {self.version}: {label}{fix}: {summary}"


def _version_key(version: str) -> Version:
    """A version string parsed for comparison. An unparseable one sorts lowest."""
    try:
        return Version(version)
    except InvalidVersion:
        return Version("0")


def scan() -> list[Finding]:
    """Runs pip-audit over this interpreter's environment and returns its findings."""
    proc = subprocess.run(
        [sys.executable, "-m", "pip_audit", "--format", "json",
         "--progress-spinner", "off", "--skip-editable"],
        cwd=ROOT, capture_output=True, text=True,
    )
    # pip-audit exits 1 when it finds vulnerabilities, so read stdout either way.
    if not proc.stdout.strip():
        raise RuntimeError(f"pip-audit produced no output (exit {proc.returncode}):\n{proc.stderr}")
    report = json.loads(proc.stdout)
    findings = [
        Finding(
            package=dep["name"],
            version=dep["version"],
            id=vuln["id"],
            aliases=tuple(vuln.get("aliases") or ()),
            fix_versions=tuple(vuln.get("fix_versions") or ()),
            description=vuln.get("description") or "",
        )
        for dep in report["dependencies"]
        for vuln in dep.get("vulns") or ()
    ]
    # The OSV and PyPI records of one advisory both appear. Keep the first.
    unique: dict[tuple[str, str], Finding] = {}
    for finding in findings:
        unique.setdefault((finding.package, finding.id), finding)
    return list(unique.values())


def load_allowlist() -> dict[str, str]:
    """The allowlisted advisory ids, mapped to their reasons."""
    if not ALLOWLIST_PATH.exists():
        return {}
    data = json.loads(ALLOWLIST_PATH.read_text())
    return {entry["id"]: entry.get("reason", "") for entry in data.get("allowlist", [])}


def is_allowed(finding: Finding, allowed: dict[str, str]) -> bool:
    """Whether the allowlist names the advisory's id or one of its aliases."""
    return finding.id in allowed or any(alias in allowed for alias in finding.aliases)


def classify(
    findings: list[Finding], allowed: dict[str, str],
) -> tuple[list[Finding], list[Finding], list[Finding]]:
    """Splits findings into (needs upgrade, allowlisted but now fixed, needs triage)."""
    upgrade, now_fixed, triage = [], [], []
    for finding in findings:
        if finding.fix_versions:
            (now_fixed if is_allowed(finding, allowed) else upgrade).append(finding)
        elif not is_allowed(finding, allowed):
            triage.append(finding)
    return upgrade, now_fixed, triage


def _normalize(name: str) -> str:
    """A package name in the PEP 503 normal form."""
    return re.sub(r"[-_.]+", "-", name).lower()


def _raise_requirement_floor(package: str, floor: str) -> bool:
    """Raises a package's `>=` floor in requirements.txt. Returns whether a line changed."""
    pattern = re.compile(r"^([A-Za-z0-9_.\-]+)(\[[^\]]*\])?>=([^\s#,;]+)")
    lines = REQUIREMENTS_PATH.read_text().splitlines(keepends=True)
    changed = False
    for i, line in enumerate(lines):
        match = pattern.match(line)
        if not match or _normalize(match.group(1)) != _normalize(package):
            continue
        if _version_key(match.group(3)) >= _version_key(floor):
            continue
        lines[i] = line[: match.start(3)] + floor + line[match.end(3):]
        changed = True
    if changed:
        REQUIREMENTS_PATH.write_text("".join(lines))
    return changed


def _installed_dependents(package: str) -> dict[str, str]:
    """The installed distributions that require a package, mapped to their versions."""
    dependents: dict[str, str] = {}
    for dist in distributions():
        for spec in dist.requires or ():
            try:
                req = Requirement(spec)
            except InvalidRequirement:
                continue
            if _normalize(req.name) == _normalize(package):
                dependents[dist.metadata["Name"]] = dist.version
                break
    return dependents


def _upgrade(findings: list[Finding]) -> None:
    """Installs each package's highest fix, then raises its requirements.txt floor."""
    targets: dict[str, str] = {}
    for finding in findings:
        current = _version_key(targets.get(finding.package, "0"))
        if finding.fix_versions and _version_key(finding.max_fix) > current:
            targets[finding.package] = finding.max_fix
    if not targets:
        print("No fixable vulnerabilities.")
        return
    # Holding each dependent at its installed version makes pip honor its version caps.
    target_names = {_normalize(target) for target in targets}
    held = {
        name: version
        for pkg in targets
        for name, version in _installed_dependents(pkg).items()
        if _normalize(name) not in target_names
    }
    subprocess.run(
        [sys.executable, "-m", "pip", "install", "--quiet", "--upgrade",
         *(f"{pkg}>={fix}" for pkg, fix in sorted(targets.items())),
         *(f"{name}=={version}" for name, version in sorted(held.items()))],
        cwd=ROOT, check=True,
    )
    subprocess.run([sys.executable, "-m", "pip", "check"], cwd=ROOT, check=True)
    print("Upgraded:")
    for pkg, fix in sorted(targets.items()):
        note = " (requirements.txt floor raised)" if _raise_requirement_floor(pkg, fix) else ""
        print(f"  {pkg} >= {fix}{note}")


def _triage(findings: list[Finding]) -> bool:
    """Asks whether to allowlist each unfixed finding. Returns whether any was declined."""
    allowed = load_allowlist()
    new: list[dict[str, str]] = []
    declined = False
    seen: set[str] = set()
    for finding in findings:
        if finding.fix_versions or is_allowed(finding, allowed) or finding.id in seen:
            continue
        seen.add(finding.id)
        print(f"\n{finding.line()}")
        if input(f"  Append to {ALLOWLIST_PATH.relative_to(ROOT)}? [y/N] ").strip().lower() == "y":
            new.append({"id": finding.id, "reason": input("  Reason: ").strip()})
        else:
            print(f"  Skipped. The audit test keeps failing on {finding.id}.")
            declined = True
    if new:
        data = (json.loads(ALLOWLIST_PATH.read_text()) if ALLOWLIST_PATH.exists()
                else {"allowlist": []})
        data["allowlist"] = data.get("allowlist", []) + new
        ALLOWLIST_PATH.write_text(json.dumps(data, indent=2) + "\n")
        print(f"\nAdded {len(new)} allowlist entr{'y' if len(new) == 1 else 'ies'}.")
    return declined


def main() -> int:
    """Upgrades, re-scans, and triages. Returns 1 while any finding still fails the gate."""
    _upgrade(scan())
    findings = scan()
    upgrade, now_fixed, _ = classify(findings, load_allowlist())
    # A parent's version cap can hold a package below its fix. The allowlist takes
    # only unfixed findings, so a capped one fails until the parent lifts the cap.
    residue = upgrade + now_fixed
    if residue:
        print("Still fixable after upgrade (a dependent's version cap may hold it back):",
              file=sys.stderr)
    for finding in residue:
        print(finding.line(), file=sys.stderr)
    declined = _triage(findings)
    if residue or declined:
        return 1
    print("\nDone. Verify with: pytest tests/test_vuln_audit.py")
    return 0


if __name__ == "__main__":
    sys.exit(main())
