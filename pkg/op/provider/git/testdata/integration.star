# Integration test for git provider.
# git_prov, test_repo_url, and test_clone_dest are injected by the Go test.
# Exercises: clone, checkout, pull.

# --- clone ---
result_clone = git_prov.clone(test_repo_url, test_clone_dest)

# --- checkout ---
result_checkout = git_prov.checkout(result_clone, "main")

# --- pull ---
result_pull = git_prov.pull(result_clone)

# Signal completion.
result_done = True
