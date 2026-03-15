# test_write_and_read.star — Write then read the same path.
# Both nodes target the same path. Nodes without edges are sorted by
# insertion order within the same path depth, so write runs first.
dest = t.tmp("readback.txt")
plan.file.write_text(destination=dest, content="read me back", mode=0o644)
plan.file.read_text(resource=dest)
t.expect_file(dest, content="read me back")
t.expect_node_count(2)
