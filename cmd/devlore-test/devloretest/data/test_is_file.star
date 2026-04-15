# test_is_file.star — Write a file and use is_file predicate in choose.
#
# 1. Write a file
# 2. Check is_file (should be true) → then-branch copies it
#
# Validates: plan.file.write_text, plan.file.is_file, plan.choose, plan.file.copy

src = t.tmp("is_file_src.txt")
dst = t.tmp("is_file_dst.txt")

written = plan.file.write_text(destination_path=src, content="file check", mode=0o644)

file_check = plan.file.is_file(resource=src)
plan.choose(
    when=file_check,
    then=lambda: plan.file.copy(source=written, destination_path=dst, mode=0o644),
)

t.expect_file(dst, content="file check")
t.expect_node_count(4)  # write_text + is_file + copy + choose
