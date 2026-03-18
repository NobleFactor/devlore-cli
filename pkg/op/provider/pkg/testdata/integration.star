# Integration test for pkg provider.
# pkg and test_packages are injected by the Go test.
# Exercises: installed, not_installed, update.

# --- installed (should return True for pre-installed package) ---
result_installed = pkg.installed("curl")

# --- not_installed (should return True for missing package) ---
result_not_installed = pkg.not_installed("nonexistent-pkg-12345")

# --- update ---
result_update = pkg.update("")

# Signal completion.
result_done = True
