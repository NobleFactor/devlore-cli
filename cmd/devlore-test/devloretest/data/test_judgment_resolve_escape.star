# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_resolve_escape.star — judgment scenario: resolve refuses the escaping chain (docs/plans/
# resource-construction.md, explicit-conversion suite item 8, second direction).
#
# A symbolic link is the disk's "../": its target lies wherever it says, and the follow is judged like
# any other traversal — a chain designating anything outside the run's root refuses with the confinement
# verdict, never a raw I/O error. The link's target here is "..": the run root's own parent, which
# always exists and is always outside.

link = t.tmp("escape-hatch")
dst = t.tmp("never.txt")

linked = plan.file.link(source_path="..", target_path=link)
designated = plan.file.resolve(path=link, after=linked)
copied = plan.file.copy(source=designated, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([linked, designated, copied])

t.expect_error("outside the run's root")
t.expect_no_file(dst)

t.run(graph)
