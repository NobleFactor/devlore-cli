# test_imm_starstats.star — Immediate Starlark file statistics.
#
# Validates: starstats.compute_stats (callable with empty input)

result = starstats.compute_stats(files=[], with_bytes=True, with_loc=True)
t.expect_unit_count(0)
