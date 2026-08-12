# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_wait_until.star — plan.wait_until polls a body subgraph until its result is truthy.
#
# The body takes the same shapes case bodies take (phase-8 step 12): a singleton invocation — any action works as the
# predicate, its result evaluated for truthiness — or a lambda desugared to the function.call leaf. The wait_until
# result is the truthy poll's result, so it flows to consumers like any unit result.
#
# Scenarios:
#   1. Invocation body: a prior rooted writer creates the file; wait_until on plan.file.exists succeeds first poll.
#   2. Lambda body: the lambda's truthy value ("ready") is the wait_until result, captured by a downstream write.

ready  = t.tmp("ready.txt")
status = t.tmp("wait_status.txt")

writer     = plan.file.write_text(destination_path=ready, content="up", chmod=0o644)
wait_ready = plan.wait_until(body=plan.file.exists(path=ready), timeout="10s", interval="1s")
wait_value = plan.wait_until(body=lambda: "ready", timeout="10s", interval="1s")
status_inv = plan.file.write_text(destination_path=status, content=wait_value, chmod=0o644)

graph = plan.assemble_definition([writer, wait_ready, wait_value, status_inv])

t.expect_file(status, content="ready")
t.expect_unit_count(6)  # nodes: writer + exists + function.call + status_write; subgraphs: 2 wait_until containers

t.run(graph)
