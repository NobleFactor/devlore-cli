# test_imm_starcode.star — Immediate Starlark source capture.
#
# Validates: starcode.capture (callable with no-match pattern)

result = starcode.capture(pattern="*.nonexistent_extension_xyz")
t.expect_unit_count(0)
