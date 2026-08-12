# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_flow_fatal.star — Verify plan.failed halts execution via the TransitionPolicy stop reaction.

t.expect_error("flow.failed executed: database unreachable")

graph = plan.assemble_definition([
    plan.failed("database unreachable"),
])

t.run(graph)
