# test_compensation.star — Write a file, then trigger a failing copy.
# RunPhased compensation should undo the write, removing the file.

dest = t.tmp("compensated.txt")
written = plan.file.write_text(destination=dest, content="should be undone", mode=0o644)

# Copy using the write output as source (creates an edge for ordering),
# but target a read-only path that will fail.
plan.file.copy(source_file=written, destination_filename="/dev/null/impossible/path.txt", destination_file_mode=0o644)

# After compensation, the written file should be removed.
t.expect_no_file(dest)
t.expect_error("file.copy")
