# test_write_text.star — Verify plan.file.write_text creates a file with correct content.
dest = t.tmp("hello.txt")
plan.file.write_text(destination_path=dest, content="hello world", chmod=0o644)
t.expect_file(dest, content="hello world")
t.expect_node_count(1)
