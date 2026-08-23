# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_dispatch_miss.star — judgment scenario: the second wall (docs/plans/resource-construction.md,
# explicit-conversion suite item 2b).
#
# A run-delivered string that misses the run catalog fails at the dispatch seam with the §5.6 verdict —
# a string is a key, never a constructor. The miss arises in-model: a gather item record carries a raw
# path string into the copy's resource-typed source (the item frame is exempt from the plan-time variable
# refusal precisely because its records MAY carry claimed resources — this scenario pins the backstop for
# the records that do not). Nothing is constructed, and the destination is never created.

dst = t.tmp("never.txt")

copied = plan.file.copy(
    source=plan.item("source"), destination_path=plan.item("dest"), mode=0o600)
gathered = plan.gather(items=[{"source": "ghost.txt", "dest": dst}], limit=1, body=[copied])

graph = plan.assemble_definition([gathered])

t.expect_error("not in the run catalog")
t.expect_no_file(dst)

t.run(graph)
