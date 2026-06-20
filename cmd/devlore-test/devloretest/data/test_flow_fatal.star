# test_flow_fatal.star — Verify plan.failed halts execution.

t.expect_error("fatal: database unreachable")

graph = plan.assemble_definition([
    plan.failed("database unreachable"),
])

t.run(graph)
