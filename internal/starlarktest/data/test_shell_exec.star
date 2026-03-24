# test_shell_exec.star — Run a shell command that creates a file, verify it exists.
#
# Validates: plan.shell.exec with side effects visible to expectations

dest = t.tmp("shell_output.txt")

plan.shell.exec(command="printf 'from shell' > " + dest)

t.expect_file(dest, content="from shell")
t.expect_node_count(1)
