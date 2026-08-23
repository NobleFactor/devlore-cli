# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_resolve_dangling.star — judgment scenario: resolve refuses the broken chain (docs/plans/
# resource-construction.md, explicit-conversion suite item 8, first direction).
#
# A dangling chain is the resolve node's own error — Stop-only, no tolerance. The link is created
# against a real target which a mutator then destroys; the resolve that follows meets the dangle.

target = t.tmp("doomed.txt")
link = t.tmp("the-link")
dst = t.tmp("never.txt")

t.write(target, "about to vanish")

linked = plan.file.link(source_path=target, target_path=link)
removed = plan.file.remove(target=target, prune=False, boundary="")
designated = plan.file.resolve(path=link, after=linked)
copied = plan.file.copy(source=designated, destination_path=dst, mode=0o600)

# Positional order carries remove between link and resolve (no data edges contradict it).
graph = plan.assemble_definition([linked, removed, designated, copied])

t.expect_error("file.resolve")
t.expect_no_file(dst)

t.run(graph)
