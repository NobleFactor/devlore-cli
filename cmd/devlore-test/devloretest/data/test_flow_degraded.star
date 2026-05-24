# test_flow_degraded.star — Verify plan.degraded creates a warning node.
# Graph should complete successfully (degraded is not an error).

graph = plan.assemble([
    plan.degraded("disk space low"),
])

t.expect_unit_count(1)
