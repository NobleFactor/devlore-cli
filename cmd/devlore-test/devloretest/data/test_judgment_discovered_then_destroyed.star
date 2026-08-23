# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_discovered_then_destroyed.star — judgment scenario: discoveries join the mediation model
# (docs/plans/resource-construction.md, explicit-conversion suite item 11).
#
# Discover interns; a typed remove destroys the entry with the destroyer stamp; the discovery's consumer
# fails on the narrated guard verdict — "destroyed by" — never on its own I/O. Discoveries get the same
# protection claims do: falseness is a mediation failure, and the mediation sees this one.

data = t.tmp("data.txt")
dst = t.tmp("never.txt")

t.write(data, "observed then destroyed")

found = plan.file.discover(path=data, kind="regular")
removed = plan.file.remove(target=data, prune=False, boundary="")
copied = plan.file.copy(source=found, destination_path=dst, mode=0o600)

# Positional order carries the remove between the discovery and its consumer.
graph = plan.assemble_definition([found, removed, copied])

t.expect_error("destroyed by")
t.expect_no_file(dst)

t.run(graph)
