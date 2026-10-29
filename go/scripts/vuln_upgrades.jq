# SPDX-License-Identifier: MIT
# Prints "module<TAB>fixed_version" for each reachable finding that has a fix.
# vulnState in tests/static_analysis_test.go defines reachable and fixable.
select(.finding != null
       and (.finding.fixed_version // "") != ""
       and .finding.trace[0].function != null)
| "\(.finding.trace[0].module)\t\(.finding.fixed_version)"
