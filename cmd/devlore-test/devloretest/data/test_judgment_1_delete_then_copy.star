# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_1_delete_then_copy.star — judgment scenario 1 (docs/plans/resource-construction.md).
#
# Two operations against the same named file, no promise between them: delete it, then copy from it. The
# prediction, authored before implementation: the stored catalog carries exactly one pending file entry with
# two consumers (ruled 2026-08-20: mutation targets are resource-typed consumers; until #585 migrates
# file.remove's path-typed signature, the entry is minted by the copy's source alone and the count is the
# same); pre-flight passes because intent was satisfied at the starting line; the run fails at the copy — under
# the ruled shape the copy sees the catalog's Gone verdict (the remove, as consumer, transitioned the entry);
# until #585 it rediscovers the loss through its own I/O; the failure unwinds and the delete's receipt restores
# the file. The graph says what
# must be true; the trace says what happened; the gap between them is the run's story.

src = t.tmp("data.txt")
dst = t.tmp("copy.txt")
doc = t.tmp("graph.json")

t.write(src, "the original bytes")

deleted = plan.file.remove(path=src, prune=False, boundary="")
copied  = plan.file.copy(source=src, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([deleted, copied])
plan.save_definition(graph, doc)

# The stored catalog is input intent: exactly one entry, the source, pending; the destination — a product —
# is absent.
document = json.decode(data=file.read_text(resource=doc))
entries = document["resources"]
t.expect_equal(len([e for e in entries if "data.txt" in str(e) and e["state"] == "pending"]), 1)
t.expect_equal(len([e for e in entries if "copy.txt" in str(e)]), 0)

# The run: the copy fails on the gone source; compensation restores the deleted file; the never-produced
# destination does not exist.
t.expect_error("file.copy")
t.expect_file(src, content="the original bytes")
t.expect_no_file(dst)

t.run(graph)
