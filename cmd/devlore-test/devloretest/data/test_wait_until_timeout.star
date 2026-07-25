# test_wait_until_timeout.star — plan.wait_until fails with a plain error when the budget expires.
#
# The body polls plan.file.exists on a path nothing creates; every poll is falsy, the 150ms budget expires across
# multiple 50ms-interval polls, and the unit fails with the timeout error (settled 2026-07-02: plain error carrying
# the poll count and the last falsy result; falsy polls leave nothing on the stack).

never = t.tmp("never_created.txt")

waited = plan.wait_until(body=plan.file.exists(path=never), timeout="150ms", interval="50ms")

graph = plan.assemble_definition([waited])

t.expect_error("timeout after")

t.run(graph)
