# test_file_unlink.star — Write a file, symlink it, then unlink the symlink.
#
# Validates: plan.file.write_text, plan.file.link, plan.file.unlink

target = t.tmp("unlink_target.txt")
link   = t.tmp("unlink_link.txt")

written = plan.file.write_text(destination_path=target, content="keep me", mode=0o644)
linked  = plan.file.link(source=written, target_path=link)
plan.file.unlink(path=linked, prune=False, boundary="")

t.expect_file(target, content="keep me")
t.expect_no_file(link)
t.expect_node_count(3)
