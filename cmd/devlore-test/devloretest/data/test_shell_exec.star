# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_shell_exec.star — Run a shell command that creates a file, verify it exists.
#
# Validates: plan.shell.exec with side effects visible to expectations

dest = t.tmp("shell_output.txt")

# The destination is single-quoted. shell.exec runs `sh -c`, on every platform, so an unquoted path is
# subject to shell escaping: on Windows t.tmp returns a native path and sh consumes its backslashes, turning
# C:\Users\... into C:Users... and writing the file somewhere else entirely.
graph = plan.assemble_definition([
    plan.shell.exec(command="printf 'from shell' > '" + dest + "'"),
])

t.expect_file(dest, content="from shell")
t.expect_unit_count(1)

t.run(graph)
