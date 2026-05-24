# test_flow_fatal.star — Verify plan.failed halts execution.

t.expect_error("fatal: database unreachable")

graph = plan.assemble([
    plan.failed("database unreachable"),
])
