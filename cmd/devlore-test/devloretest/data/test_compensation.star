# test_compensation.star — Write a file, then trigger a failing copy.
# RunPhased compensation should undo the write, removing the file.

dest = t.tmp("compensated.txt")

written = plan.file.write_text(destination_path=dest, content="should be undone", chmod=0o644)

# Copy using the write output as source (creates an edge for ordering),
# but target a read-only path that will fail.
copied = plan.file.copy(source=written, destination_path="/dev/null/impossible/path.txt", chmod=0o644)

graph = plan.assemble_definition([written, copied])

# After compensation, the written file should be removed.
t.expect_no_file(dest)
t.expect_error("file.copy")

t.run(graph)
