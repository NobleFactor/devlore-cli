# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_starcomplexity.star — Immediate Starlark complexity analysis.
#
# Validates: starcomplexity.compute_complexity (callable with empty input)

result = starcomplexity.compute_complexity(files=[])
t.expect_unit_count(0)
