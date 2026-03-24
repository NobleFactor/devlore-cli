# test_file_join.star — Join path components via planned action.
#
# Validates: plan.file.join (creates a graph node for a pure function)

plan.file.join(parts=["a", "b", "c.txt"])
t.expect_node_count(1)
