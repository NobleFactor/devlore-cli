# test_flow_fatal.star — Verify plan.fatal halts execution.

t.expect_error("fatal: database unreachable")
plan.fatal("database unreachable")
