# test_flow_fatal_recovery.star — Verify compensable actions before fatal are unwound.
# write_text is compensable; after fatal halts, the file should be removed.

dest = t.tmp("to-be-undone.txt")

written = plan.file.write_text(destination_path=dest, content="temporary", chmod=0o644)
fatal   = plan.failed("abort after write")

graph = plan.assemble_definition([written, fatal])

t.expect_error("fatal: abort after write")
t.expect_no_file(dest)

t.run(graph)
