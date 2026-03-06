# test_imm_shell.star — Immediate shell execution.
#
# Validates: shell.exec (immediate mode)
# shell.exec returns the command string, not stdout.

result = shell.exec(command="echo hello")
t.expect_equal(result, "echo hello")

t.expect_node_count(0)
