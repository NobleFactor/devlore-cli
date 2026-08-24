# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_remove_any_kind.star — judgment scenario: the removal family splits by blast radius,
# never by kind (docs/plans/any-entry-claims.md, phase 5).
#
# `file.remove` takes one entry of any kind; `file.remove_all` takes an entry and everything beneath it.
# That is the standard library's split — os.Remove tries unlink then rmdir rather than asking the kind,
# which is why Go never needed an Unlink, and os.RemoveAll opens by calling os.Remove.
#
# Two sharp assertions:
#   - `remove` over a symbolic link takes the LINK; its target survives.
#   - `remove_all` over a link to a POPULATED directory also takes only the link; the tree survives.
#     A follow would be catastrophic and unmistakable — every file under it would vanish.

regular = t.tmp("a-regular.txt")
empty_dir = t.tmp("an-empty-directory")
link_target = t.tmp("link-target.txt")
the_link = t.tmp("the-link")

populated = t.tmp("populated")
populated_child = t.tmp("populated/child.txt")
link_to_tree = t.tmp("link-to-tree")

t.write(regular, "removed as an entry")
t.mkdir(empty_dir)
t.write(link_target, "the link's target survives")
t.symlink(link_target, the_link)
t.mkdir(populated)
t.write(populated_child, "the tree survives a link removal")
t.symlink(populated, link_to_tree)

graph = plan.assemble_definition([
    plan.file.remove(target=regular, prune=False, boundary=""),
    plan.file.remove(target=empty_dir, prune=False, boundary=""),
    plan.file.remove(target=the_link, prune=False, boundary=""),
    plan.file.remove_all(target=link_to_tree, prune=False, boundary=""),
])

# All four entries are gone.
t.expect_no_file(regular)
t.expect_no_file(empty_dir)
t.expect_no_file(the_link)
t.expect_no_file(link_to_tree)

# Neither removal followed: both targets are untouched, tree and all.
t.expect_file(link_target, content="the link's target survives")
t.expect_file(populated_child, content="the tree survives a link removal")

t.run(graph)
