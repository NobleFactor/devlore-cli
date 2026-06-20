# test_source.star — Use plan.file.read_text to read an existing file.
#
# 1. Write a file (via shell to avoid plan.file edge coupling)
# 2. Read it back via plan.file.read_text
#
# Validates: plan.file.read_text

dest = t.tmp("source_input.txt")

# Use shell to create the file outside the graph's file provider,
# so plan.file.read_text reads it independently.
graph = plan.assemble_definition([
    plan.shell.exec(command="printf 'source test' > " + dest),
    plan.file.read_text(resource=dest),
])

t.expect_file(dest, content="source test")
t.expect_unit_count(2)  # shell.exec + file.read_text

t.run(graph)
