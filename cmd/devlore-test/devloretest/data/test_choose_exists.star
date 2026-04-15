# test_choose_exists.star — Exercise plan.choose with file.exists predicate.
#
# 1. Write a file
# 2. Check existence (should be true) → choose then-branch removes it
# 3. Check existence again (should be false) → choose is a no-op
#
# Validates: plan.file.write_text, plan.file.exists, plan.choose, plan.file.remove

dest = t.tmp("choose_target.txt")

# Step 1: Create the file.
plan.file.write_text(destination_path=dest, content="delete me", mode=0o644)

# Step 2: Check existence — should be true, so the then-branch fires.
exists_output = plan.file.exists(resource=dest)
plan.choose(
    when=exists_output,
    then=lambda: plan.file.remove(path=dest, prune=False, boundary=""),
)

# Step 3: After removal, the file should be gone.
t.expect_no_file(dest)
t.expect_node_count(4)  # write_text + exists + remove + choose
