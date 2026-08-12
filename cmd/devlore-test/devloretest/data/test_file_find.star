# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_file_find.star — Create files and find them recursively.
#
# Validates: plan.file.mkdir, plan.file.write_text, plan.file.find

dir = t.tmp("finddir")
graph = plan.assemble_definition([
    plan.file.mkdir(path=dir, chmod=0o755),
    plan.file.write_text(destination_path=t.tmp("finddir/a.txt"), content="a", chmod=0o644),
    plan.file.write_text(destination_path=t.tmp("finddir/b.txt"), content="b", chmod=0o644),
    plan.file.find(pattern=t.tmp("finddir/*.txt"), include_gitignored=True),
])

t.expect_file(t.tmp("finddir/a.txt"), content="a")
t.expect_file(t.tmp("finddir/b.txt"), content="b")
t.expect_unit_count(4)  # mkdir + write_text + write_text + find

t.run(graph)
