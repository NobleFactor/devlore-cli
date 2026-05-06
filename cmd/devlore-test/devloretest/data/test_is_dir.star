# test_is_dir.star — Create a directory and use is_dir predicate in choose.
#
# 1. Create directory via mkdir
# 2. Check is_dir (should be true) → then-branch writes a file inside it
#
# Validates: plan.file.mkdir, plan.file.is_dir, plan.choose

dir  = t.tmp("is_dir_test")
file = t.tmp("is_dir_test/proof.txt")

plan.file.mkdir(path=dir, chmod=0o755)

dir_check = plan.file.is_dir(resource=dir)
plan.choose(
    when=dir_check,
    then=lambda: plan.file.write_text(destination_path=file, content="dir exists", chmod=0o644),
)

t.expect_file(file, content="dir exists")
t.expect_node_count(4)  # mkdir + is_dir + write_text + choose
