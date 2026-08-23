# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_reload_dispatch.star — judgment scenario: save, reload, run (docs/plans/resource-construction.md,
# explicit-conversion suite item 1).
#
# The prediction, authored from the §5.6 ruling: a reloaded graph's resource-typed slot carries the URI
# string the save left behind; at graph dispatch that string is a KEY — it resolves through the
# section-rehydrated run catalog to the claimed entry, and the copy flows. No construction happens at
# dispatch; the object-identity half of the pin lives in Go (resolveDispatchResource's canonical-pointer
# assertions and the pristine-location pin).

src = t.tmp("original.txt")
dst = t.tmp("copied.txt")
doc = t.tmp("graph.json")

t.write(src, "identity flows")

copied = plan.file.copy(source=src, destination_path=dst, mode=0o600)
graph = plan.assemble_definition([copied])
plan.save_definition(graph, doc)

loaded = plan.load_definition(doc)

t.expect_file(dst, content="identity flows")

t.run(loaded)
