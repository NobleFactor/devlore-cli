# test_file_parent.star — Extract parent directory via planned action.
#
# Validates: plan.file.parent (creates a graph node for a pure function)

plan.file.parent(path="/some/dir/file.txt")
t.expect_node_count(1)
