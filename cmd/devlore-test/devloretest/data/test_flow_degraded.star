# test_flow_degraded.star — Verify plan.degraded creates a warning node.
# Graph should complete successfully (degraded is not an error).

plan.degraded("disk space low")

t.expect_node_count(1)
