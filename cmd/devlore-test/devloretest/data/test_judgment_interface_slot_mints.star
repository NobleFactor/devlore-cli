# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_interface_slot_mints.star — judgment scenario: an interface-typed slot mints the
# designated claim (docs/plans/any-entry-claims.md, phase 3).
#
# Replaces the refusal this scenario used to pin. Before the designation, an authored string bound to an
# interface-typed resource slot was refused outright: a claim asserts a kind and an interface asserts
# none. The interface now names the claim that deliberately asserts nothing on its behalf — `file.Resource`
# designates `*file.Any` — so the same authored path claims, and resolves to whatever the disk holds at
# activation.
#
# The narrowed refusal (an interface that designates NO claim type) is unreachable from Starlark, because
# every announced resource-interface parameter is `file.Resource`; it is pinned in Go
# (TestActionPlanner_AnUndesignatedResourceInterfaceRefusesAnAuthoredString).

observed = t.tmp("observed.txt")
doc = t.tmp("graph.json")

t.write(observed, "seen through an interface-typed slot")

# file.observe takes the INTERFACE. Before the designation this line refused at plan time.
seen = plan.file.observe(resource=observed)

graph = plan.assemble_definition([seen])
plan.save_definition(graph, doc)

# The claim is real intent: one entry, and its type fragment names the unasserted claim — the graph says
# "something must be here", and the trace will say what was found.
document = json.decode(data=file.read_text(resource=doc))
entries = [e for e in document["resources"] if "observed.txt" in str(e)]
t.expect_equal(len(entries), 1)
t.expect_equal("file.Any" in str(entries[0]), True)

t.run(graph)
