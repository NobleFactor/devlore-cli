# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_copy.star — Verify plan.file.copy duplicates a file to a new path.

# Write a source file first. The Output is passed to copy's source
# parameter, creating an edge that guarantees execution order.
src = t.tmp("source.txt")
dst = t.tmp("destination.txt")

written = plan.file.write_text(destination_path=src, content="copy me", mode=0o644)
copied  = plan.file.copy(source=written, destination_path=dst, mode=0o644)

graph = plan.assemble_definition([written, copied])

t.expect_file(dst, content="copy me")
t.expect_unit_count(2)

t.run(graph)
