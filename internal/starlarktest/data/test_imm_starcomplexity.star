# test_imm_starcomplexity.star — Immediate Starlark complexity analysis.
#
# Validates: starcomplexity.compute_complexity (callable with empty input)

result = starcomplexity.compute_complexity(files=[])
t.expect_node_count(0)
