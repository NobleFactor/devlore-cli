# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_gone_tolerance.star — judgment scenario: gone tolerance (docs/plans/resource-construction.md).
#
# Two removes of the same file, in order, with no promise between them. The first consumes and destroys the
# resource (Gone, destroyer-stamped). The second declares on_missing="ignore" — the guard at the dispatch
# seam sees the catalog's Gone state, warns, and MAKES THE CALL; the provider no-ops on the absence. The
# run succeeds and the trace's story is one deletion plus one recorded no-op. Under the default (stop) the
# second remove would fail on the narrated verdict — that direction is pinned by judgment scenario 1. (A
# Skip variant was considered and dropped, ruled 2026-08-22.)

f = t.tmp("twice.txt")
t.write(f, "delete me once")

first = plan.file.remove(target=f, prune=False, boundary="")
second = plan.file.remove(target=f, on_missing="ignore", prune=False, boundary="")

graph = plan.assemble_definition([first, second])

t.expect_no_file(f)
t.expect_unit_count(2)

t.run(graph)
