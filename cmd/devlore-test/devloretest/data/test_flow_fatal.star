# test_flow_fatal.star — Verify plan.flow.fatal halts execution.

t.expect_error("fatal: database unreachable")
plan.flow.fatal("database unreachable")
