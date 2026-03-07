# test_flow_fatal_recovery.star — Verify compensable actions before fatal are unwound.
# write_text is compensable; after fatal halts, the file should be removed.

dest = t.tmp("to-be-undone.txt")
plan.file.write_text(destination=dest, content="temporary", mode=0o644)
plan.flow.fatal("abort after write")

t.expect_error("fatal: abort after write")
t.expect_no_file(dest)
