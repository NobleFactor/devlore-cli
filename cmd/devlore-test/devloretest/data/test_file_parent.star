# test_file_parent.star — Extract parent directory via planned action.
#
# Validates: plan.file.parent (creates a graph node for a pure function)

graph = plan.assemble([
    plan.file.parent(path="/some/dir/file.txt"),
])
t.expect_unit_count(1)
