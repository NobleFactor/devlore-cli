# test_flow_fatal.star — Verify plan.fatal halts execution.

t.expect_error("fatal: database unreachable")

graph = plan.assemble([
    plan.fatal("database unreachable"),
])
