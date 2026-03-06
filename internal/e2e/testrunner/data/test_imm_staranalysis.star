# test_imm_staranalysis.star — Immediate Starlark analysis.
#
# Validates: staranalysis.analyze (callable with empty input)

result = staranalysis.analyze(files=[])
t.expect_node_count(0)
