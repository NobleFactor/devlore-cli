# Integration test for shell provider.
# Exercises the exec method via the executing receiver.
# Sets result_* globals for the Go test to inspect.

# --- exec returns the command string ---
result_exec = shell.exec("echo hello")
result_exec_type = type(result_exec)

# Signal completion.
result_done = True
