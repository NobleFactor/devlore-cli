# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_claimed_and_discovered.star — judgment scenario: one identity, both doors (docs/plans/
# resource-construction.md, explicit-conversion suite item 10).
#
# A path claimed at plan time by one unit and discovered mid-run by another dedups to ONE catalog
# identity — the catalog mediating across the claim door and the discovery door. The stored graph
# carries exactly the one claim (a discovery is a runtime fact and adds nothing to intent); at run time
# the discovery reaches the claimed, already-verified entry, and the consumer of its promise reads
# through the same identity.

data = t.tmp("data.txt")
dst = t.tmp("copied.txt")
doc = t.tmp("graph.json")

t.write(data, "one identity")

read = plan.file.read_text(resource=data)
found = plan.file.discover(path=data, kind="regular", after=read)
copied = plan.file.copy(source=found, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([read, found, copied])
plan.save_definition(graph, doc)

# The stored intent: exactly one entry — the claim; the discovery contributes nothing at plan time.
document = json.decode(data=file.read_text(resource=doc))
t.expect_equal(len(document["resources"]), 1)

t.expect_file(dst, content="one identity")

t.run(graph)
