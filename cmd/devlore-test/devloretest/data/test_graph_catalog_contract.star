# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_graph_catalog_contract.star — the serialized graph carries the catalog as input intent
# (4-resource-management.md §5, ruled 2026-08-20; judgment scenarios in docs/plans/resource-construction.md).
#
# The contract: the graph's resource catalog represents what must exist when the graph runs. file.copy is the
# exemplar because its signature spans both grammars: `source` is resource-typed (a string value mints a
# pending entry), `destination_path` is a plain string naming a product — a runtime fact with no plan-time
# presence.
#
# Front to back: fixture -> plan -> save -> read the document back -> judge the document.

src = t.tmp("original.txt")
dst = t.tmp("duplicate.txt")
doc = t.tmp("graph.json")

t.write(src, "bytes to copy")

node  = plan.file.copy(source=src, destination_path=dst, mode=0o600)
graph = plan.assemble_definition([node])
plan.save_definition(graph, doc)

# The end result: the artifact a later session loads.
document = json.decode(data=file.read_text(resource=doc))

# Sanity: the document is real and carries the planned node — the harness ran front to back.
t.expect_equal(len(document["nodes"]), 1)
t.expect_equal(document["nodes"][0]["action_name"], "file.copy")

# 1 — the catalog section exists: mandatory, even when empty.
t.expect_equal("resources" in document, True)

entries = document["resources"]

# 2 — the source is present, as pending intent.
t.expect_equal(len([e for e in entries if "original.txt" in str(e)]), 1)
t.expect_equal(len([e for e in entries if "original.txt" in str(e)]), 1)
t.expect_equal(len([e for e in entries if "state" in e]), 0)  # stateless rows: presence IS the pending claim

# 3 — the destination is ABSENT: a product is a runtime fact, not intent.
t.expect_equal(len([e for e in entries if "duplicate.txt" in str(e)]), 0)
