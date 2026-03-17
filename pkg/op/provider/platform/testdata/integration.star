# Integration test for platform provider.
# Exercises all five properties via the executing receiver.
# Sets result_* globals for the Go test to inspect.

# --- properties (zero-param, string-return → eager evaluation) ---
result_arch = platform.arch
result_os = platform.os
result_distro = platform.distro
result_hostname = platform.hostname
result_version = platform.version

# Signal completion.
result_done = True
