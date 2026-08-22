# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_mkdir_and_remove_all.star — Create a directory tree and remove it.
#
# 1. Create a directory with mkdir
# 2. Write a file inside it
# 3. Remove the entire tree with remove_all
#
# Validates: plan.file.mkdir, plan.file.write_text, plan.file.remove_all

dir  = t.tmp("mydir")
file = t.tmp("mydir/nested.txt")

# remove_all consumes mkdir's PROMISE: the graph itself creates the tree, so the claim rides the edge —
# the promise-less name-coincidence form is refused by scoped pre-flight, by design (§3).
made = plan.file.mkdir(path=dir, mode=0o755)
written = plan.file.write_text(destination_path=file, content="nested content", mode=0o644)

graph = plan.assemble_definition([
    made,
    written,
    plan.file.remove_all(target=made, prune=False, boundary=""),
])

t.expect_no_file(file)
t.expect_unit_count(3)

t.run(graph)
