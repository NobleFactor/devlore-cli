# test_mkdir_and_remove_all.star — Create a directory tree and remove it.
#
# 1. Create a directory with mkdir
# 2. Write a file inside it
# 3. Remove the entire tree with remove_all
#
# Validates: plan.file.mkdir, plan.file.write_text, plan.file.remove_all

dir  = t.tmp("mydir")
file = t.tmp("mydir/nested.txt")

# Step 1: Create directory.
plan.file.mkdir(path=dir, chmod=0o755)

# Step 2: Write a file inside it.
plan.file.write_text(destination_path=file, content="nested content", chmod=0o644)

# Step 3: Remove the entire directory tree.
plan.file.remove_all(path=dir, prune=False, boundary="")

t.expect_no_file(file)
t.expect_node_count(3)
