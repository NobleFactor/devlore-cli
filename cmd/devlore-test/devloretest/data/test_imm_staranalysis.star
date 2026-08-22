# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_staranalysis.star — Immediate Starlark analysis.
#
# Validates: staranalysis.analyze (callable with empty input)

result = staranalysis.analyze(files=[], cfg={})
t.expect_unit_count(0)
