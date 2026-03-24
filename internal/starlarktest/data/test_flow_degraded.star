# test_flow_degraded.star — Verify plan.flow.degraded creates a warning node.
# Graph should complete successfully (degraded is not an error).

plan.flow.degraded("disk space low")

t.expect_node_count(1)
