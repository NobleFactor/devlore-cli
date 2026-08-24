# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_move_any_kind.star — judgment scenario: file.move moves any kind (docs/plans/
# any-entry-claims.md, phase 4).
#
# The capability that regressed when kind-honest activation landed (#611): a `*Regular` claim over a
# symbolic link failed verification, so a link could not be moved at all. `file.move` now takes the
# taxonomy interface, so an authored path claims as `file.Any` and resolves to whatever the disk holds.
#
# All three kinds move in one graph, and the sharp assertion is the link: **the link moves, its target
# does not**. A follow would show up unmistakably — the target would vanish from its original path.

target = t.tmp("link-target.txt")
link = t.tmp("the-link")
regular = t.tmp("a-regular.txt")
directory = t.tmp("a-directory")
nested = t.tmp("a-directory/nested.txt")

moved_link = t.tmp("moved-link")
moved_regular = t.tmp("moved-regular.txt")
moved_directory = t.tmp("moved-directory")

t.write(target, "the link's target, which must not move")
t.write(regular, "a regular file")
t.mkdir(directory)
t.write(nested, "carried along by the rename")
t.symlink(target, link)

graph = plan.assemble_definition([
    plan.file.move(source=link, destination_path=moved_link),
    plan.file.move(source=regular, destination_path=moved_regular),
    plan.file.move(source=directory, destination_path=moved_directory),
])

# The link moved; its target stayed exactly where it was.
t.expect_no_file(link)
t.expect_file(target, content="the link's target, which must not move")

# The regular file and the whole directory subtree moved.
t.expect_no_file(regular)
t.expect_file(moved_regular, content="a regular file")
t.expect_no_file(nested)
t.expect_file(t.tmp("moved-directory/nested.txt"), content="carried along by the rename")

t.run(graph)
