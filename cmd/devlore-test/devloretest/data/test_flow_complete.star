# test_flow_complete.star — Verify plan.flow.complete creates terminal nodes.

# Complete with output value
plan.flow.complete(output=42)

# Complete with no output (nil)
plan.flow.complete()

t.expect_node_count(2)
