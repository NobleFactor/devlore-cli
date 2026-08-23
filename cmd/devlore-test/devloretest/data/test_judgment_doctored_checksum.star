# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_doctored_checksum.star — judgment scenario: the first wall (docs/plans/resource-construction.md,
# explicit-conversion suite item 2a).
#
# A hand-doctored document never reaches the dispatch seam: slots live inside the canonical bytes, so the
# integrity gate refuses the altered document at load — before any catalog, any pre-flight, any dispatch.
# The dispatch seam's own refusal (the second wall) is pinned by suite item 2b, where the miss arises
# in-model through an item record.

src = t.tmp("claimed.txt")
dst = t.tmp("never.txt")
doc = t.tmp("graph.json")
bad = t.tmp("doctored.json")

t.write(src, "present and claimed")

copied = plan.file.copy(source=src, destination_path=dst, mode=0o600)
graph = plan.assemble_definition([copied])
plan.save_definition(graph, doc)

# Doctor the document's text: every occurrence of the claimed name flips to a ghost. Any alteration of
# the canonical bytes — slot or claim row alike — breaks the recorded checksum.
t.write(bad, file.read_text(resource=doc).replace("claimed.txt", "ghost.txt"))

t.expect_error("checksum mismatch")
t.expect_no_file(dst)

plan.load_definition(bad)
