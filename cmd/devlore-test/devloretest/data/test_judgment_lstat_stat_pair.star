# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_lstat_stat_pair.star — judgment scenario: the lstat/stat pair (docs/plans/
# resource-construction.md, explicit-conversion suite item 7).
#
# One rel that is a symbolic link to a regular file: discover interns the LINK (lstat — the entry
# itself, asserted with kind="symbolic_link"), and resolve interns the REGULAR the chain designates
# (stat — the explicit follow; the interned identity is the terminus's rel). A copy fed by the
# resolution reads the target's content — the follow doctrine in one scenario.

target = t.tmp("linked-content.txt")
link = t.tmp("the-link")
dst = t.tmp("copied.txt")

t.write(target, "reached through the follow")

linked = plan.file.link(source_path=target, target_path=link)
the_link = plan.file.discover(path=link, kind="symbolic_link", after=linked)
designated = plan.file.resolve(path=link, kind="regular", after=linked)
copied = plan.file.copy(source=designated, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([linked, the_link, designated, copied])

t.expect_file(dst, content="reached through the follow")

t.run(graph)
