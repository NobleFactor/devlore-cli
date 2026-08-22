# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_discover_after_exec.star — judgment scenario: discover a mid-run fact (docs/plans/resource-construction.md).
#
# An opaque shell command writes a file at a known relative path — a side effect no promise can deliver.
# The prediction, authored from the explicit-conversion rulings (2026-08-22): the path reaches
# file.discover as a string LITERAL; discover interns the file at ITS dispatch (lstat, no follow,
# Stop-only) as a discovery — an observed fact, not intent — so the stored graph catalog carries NO claim
# for it; the consumer receives the discovered entry through the promise and reads the content the command
# wrote. The sharp assertion is the ordering edge: list position does not order execution (the
# promise-ordering scenario's proof), so the exec → discover sequencing must be an explicit edge. The
# mechanism for a pure ordering edge is the implementing PR's decision (explicit-conversion docket,
# item 6); this script gains that edge and un-skips there.

out = t.tmp("report.txt")
dst = t.tmp("copied.txt")
doc = t.tmp("graph.json")

ran = plan.shell.exec(command="printf 'written by the tool' > " + out)
# ORDERING EDGE REQUIRED: `found` must sequence after `ran` — the mechanism is decided at the
# implementing PR; as authored, nothing orders these two units.
found = plan.file.discover(path=out)
copied = plan.file.copy(source=found, destination_path=dst, mode=0o600)

# Consumer-first list: only edges may order execution.
graph = plan.assemble_definition([copied, found, ran])
plan.save_definition(graph, doc)

# The discovered path is a mid-run fact, not intent: the stored catalog carries no claim for it.
document = json.decode(data=file.read_text(resource=doc))
t.expect_equal(len(document["resources"]), 0)

t.expect_file(out, content="written by the tool")
t.expect_file(dst, content="written by the tool")

t.run(graph)
