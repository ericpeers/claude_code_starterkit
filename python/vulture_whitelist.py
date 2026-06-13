# SPDX-License-Identifier: MIT
"""
Vulture whitelist — symbols that look unused to static analysis but are
intentional. Referencing a name here makes Vulture count it as "used".

This file is consumed by ``tests/test_no_dead_code.py``. Add an entry ONLY when
the flagged symbol is genuinely intentional (public API, used by tests, used via
dynamic dispatch). If it is actually dead, delete the symbol instead. Document
*why* next to each entry, as below.

Vulture matches by name, so any plugin/dispatch pattern where every plugin
defines the same method names is already covered by name collision and does not
need whitelisting here.
"""
# ruff: noqa: F821  — bare-name references are the vulture-whitelist format, not real code

# Example entry (delete these once you have real ones):
# Public API of a utility module that has no in-repo caller yet.
# my_public_helper
