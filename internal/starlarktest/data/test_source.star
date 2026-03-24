# test_source.star — Use plan.source to read an existing file.
#
# 1. Write a file (via shell to avoid plan.file edge coupling)
# 2. Read it back via plan.source
#
# Validates: plan.source (wraps file.read_text)

dest = t.tmp("source_input.txt")

# Use shell to create the file outside the graph's file provider,
# so plan.source reads it independently.
plan.shell.exec(command="printf 'source test' > " + dest)
plan.source(path=dest)

t.expect_file(dest, content="source test")
t.expect_node_count(2)  # shell.exec + file.read_text (from source)
