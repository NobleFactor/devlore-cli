# test_file_join.star — Join path components via planned action.
#
# Validates: plan.file.join (creates a graph node for a pure function)

graph = plan.assemble_definition([
    plan.file.join(parts=["a", "b", "c.txt"]),
])
t.expect_unit_count(1)

t.run(graph)
