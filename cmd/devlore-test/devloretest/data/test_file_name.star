# test_file_name.star — Extract base name via planned action.
#
# Validates: plan.file.name (creates a graph node for a pure function)

graph = plan.assemble([
    plan.file.name(path="/some/dir/file.txt"),
])
t.expect_unit_count(1)
