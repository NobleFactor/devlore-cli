# test_move.star — Write a file and move it to a new location.
#
# Validates: plan.file.write_text, plan.file.move

src = t.tmp("move_src.txt")
dst = t.tmp("move_dst.txt")

written = plan.file.write_text(destination_path=src, content="moving data", chmod=0o644)
moved   = plan.file.move(source=written, destination_path=dst)

graph = plan.assemble([written, moved])

t.expect_no_file(src)
t.expect_file(dst, content="moving data")
t.expect_unit_count(2)
