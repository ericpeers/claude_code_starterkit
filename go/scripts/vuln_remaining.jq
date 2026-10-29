# SPDX-License-Identifier: MIT
# Prints "osv_id<TAB>fixed_version" for each reachable finding.
# vulnState in tests/static_analysis_test.go defines reachable and fixable.
select(.finding != null and .finding.trace[0].function != null)
| "\(.finding.osv)\t\(.finding.fixed_version // "")"
