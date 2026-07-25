# test_write_bytes.star — Write raw bytes to a file.
#
# Validates: plan.file.write_bytes

dest = t.tmp("bytes_output.bin")

graph = plan.assemble_definition([
    plan.file.write_bytes(destination_path=dest, content="raw bytes here", chmod=0o644),
])

t.expect_file(dest, content="raw bytes here")
t.expect_unit_count(1)

t.run(graph)
