# test_file_lifecycle.star — Full file lifecycle: write, copy, read back.
#
# 1. Write a text file
# 2. Copy it to a second location (edge from write ensures ordering)
# 3. Read the copy back (verifies the copy was written)
#
# Validates: plan.file.write_text, plan.file.copy, plan.file.read_text

src = t.tmp("lifecycle_src.txt")
dst = t.tmp("lifecycle_dst.txt")

# Step 1: Write original.
written = plan.file.write_text(destination_path=src, content="lifecycle test", chmod=0o644)

# Step 2: Copy to destination (edge from write ensures ordering).
copied = plan.file.copy(source=written, destination_path=dst, chmod=0o644)

# Step 3: Read the copy (edge from copy ensures file exists before read).
plan.file.read_text(resource=copied)

t.expect_file(src, content="lifecycle test")
t.expect_file(dst, content="lifecycle test")
t.expect_node_count(3)
