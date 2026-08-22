# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_write_and_read.star — Write then read the same path.
# The read consumes the write's PROMISE: ordering comes from the edge, and a promise-fed slot claims
# nothing — the file need not exist when the run starts because the graph itself creates it (§3: the
# promise-less name-coincidence form of this plan is refused by scoped pre-flight, by design).
dest = t.tmp("readback.txt")
written = plan.file.write_text(destination_path=dest, content="read me back", mode=0o644)
graph = plan.assemble_definition([
    written,
    plan.file.read_text(resource=written),
])
t.expect_file(dest, content="read me back")
t.expect_unit_count(2)

t.run(graph)
