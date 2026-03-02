# test_copy.star — Verify plan.file.copy duplicates a file to a new path.

# Write a source file first. The Output is passed to copy's source_file
# parameter, creating an edge that guarantees execution order.
src = t.tmp("source.txt")
written = plan.file.write_text(destination=src, content="copy me", mode=0o644)

# Copy source to destination — pass the Output as source_file for ordering.
dst = t.tmp("destination.txt")
plan.file.copy(source_file=written, destination_filename=dst, destination_file_mode=0o644)

t.expect_file(dst, content="copy me")
t.expect_node_count(2)
