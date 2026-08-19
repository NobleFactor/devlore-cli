# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_compensation.star — Write a file, then trigger a failing copy.
# RunPhased compensation should undo the write, removing the file.

dest = t.tmp("compensated.txt")

# The copy is made to fail by giving its destination a parent that is a regular file, which no platform
# will create a directory over. The previous mechanism -- a destination under "/dev/null" -- only fails
# where /dev/null exists and is not a directory: on Windows it named an ordinary path, the copy succeeded,
# compensation never ran, and the test created directories at the root of the current drive.
blocker = t.tmp("blocker")
t.write(blocker, "not a directory")

written = plan.file.write_text(destination_path=dest, content="should be undone", mode=0o644)

# Copy using the write output as source (creates an edge for ordering),
# but target a path beneath the blocker, which cannot be created.
copied = plan.file.copy(source=written, destination_path=t.tmp("blocker/child.txt"), mode=0o644)

graph = plan.assemble_definition([written, copied])

# After compensation, the written file should be removed.
t.expect_no_file(dest)
t.expect_error("file.copy")

t.run(graph)
