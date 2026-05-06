# test_write_bytes.star — Write raw bytes to a file.
#
# Validates: plan.file.write_bytes

dest = t.tmp("bytes_output.bin")

plan.file.write_bytes(destination_path=dest, content="raw bytes here", chmod=0o644)

t.expect_file(dest, content="raw bytes here")
t.expect_node_count(1)
