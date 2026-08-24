# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_file_remove_link.star — Write a file, symlink it, then remove the symlink.
#
# Was test_file_unlink.star: `file.unlink` retired into `file.remove` (2026-08-23), because the two were
# one operation — discover the entry, archive it, mint the receipt, mark it Gone — differing only in the
# claim's type. The standard library made the same call: os.Remove takes a link, which is why Go never
# needed an Unlink.
#
# The assertion that proves nothing was lost: the LINK goes and its TARGET stays. A removal never
# follows.
#
# Validates: plan.file.write_text, plan.file.link, plan.file.remove over a symbolic link.

target = t.tmp("unlink_target.txt")
link   = t.tmp("unlink_link.txt")

written = plan.file.write_text(destination_path=target, content="keep me", mode=0o644)
linked  = plan.file.link(source_path=written, target_path=link)
removed = plan.file.remove(target=linked, prune=False, boundary="")

graph = plan.assemble_definition([written, linked, removed])

t.expect_file(target, content="keep me")
t.expect_no_file(link)
t.expect_unit_count(3)

t.run(graph)
