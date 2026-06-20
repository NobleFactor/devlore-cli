# test_flow_complete.star — Verify plan.complete creates terminal nodes.

graph = plan.assemble_definition([
    plan.complete(output=42),  # Complete with output value
    plan.complete(),           # Complete with no output (nil)
])

t.expect_unit_count(2)
