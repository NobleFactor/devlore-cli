# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_file_parent.star — Extract parent directory via planned action.
#
# Validates: plan.file.parent (creates a graph node for a pure function)

graph = plan.assemble_definition([
    plan.file.parent(path="/some/dir/file.txt"),
])
t.expect_unit_count(1)

t.run(graph)
