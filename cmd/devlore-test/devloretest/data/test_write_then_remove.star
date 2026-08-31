# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_write_then_remove.star — Two operations, one edge: write a file, then remove it.
#
# The smallest graph that still has a dependency. remove consumes write_text's PROMISE, so the claim rides
# the edge rather than coinciding by name — the form scoped pre-flight accepts (§3).
#
# This is the fixture the output-format conformance test renders through every --output value, so it is
# deliberately small: two units, one file, no branching. What is being exercised is the presentation of a
# result, not the graph.

dest = t.tmp("scratch.txt")

written = plan.file.write_text(destination_path=dest, content="hello world", mode=0o644)

graph = plan.assemble_definition([
    written,
    plan.file.remove(target=written, prune=False, boundary=""),
])

t.run(graph)

t.expect_no_file(dest)
t.expect_unit_count(2)
