# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_promise_ordering.star — judgment scenario: promise ordering (docs/plans/resource-construction.md).
#
# One unit produces a file; a second consumes it through the producer's promise, and the list hands the
# consumer to assemble_definition FIRST. The prediction, authored from the ruled design: ordering comes from
# promises, so the write still runs before the copy; the promise-fed source mints NO pending entry (a promise
# is recorded, not claimed) and the products are runtime facts — the stored catalog section is present and
# EMPTY; the content flows through the promise into the copy.

out = t.tmp("produced.txt")
dst = t.tmp("copied.txt")
doc = t.tmp("graph.json")

written = plan.file.write_text(destination_path=out, content="flows through the promise", mode=0o600)
copied  = plan.file.copy(source=written, destination_path=dst, mode=0o600)

# Consumer first: if ordering were positional, the copy would dispatch before its source exists.
graph = plan.assemble_definition([copied, written])
plan.save_definition(graph, doc)

# The mandatory catalog section is present and empty: no claims, only a promise and two products.
document = json.decode(data=file.read_text(resource=doc))
t.expect_equal(len(document["resources"]), 0)

t.expect_unit_count(2)
t.expect_file(out, content="flows through the promise")
t.expect_file(dst, content="flows through the promise")

t.run(graph)
