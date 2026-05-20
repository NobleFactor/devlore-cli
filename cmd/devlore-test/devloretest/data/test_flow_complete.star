# test_flow_complete.star — Verify plan.complete creates terminal nodes.

# Complete with output value
plan.complete(output=42)

# Complete with no output (nil)
plan.complete()

t.expect_unit_count(2)
