# test_git.star — Dry-run: git actions create graph nodes.
#
# Validates: plan.git.clone, plan.git.checkout, plan.git.pull (registration + node creation)

cloned = plan.git.clone(url="https://example.com/repo.git", destination="/tmp/repo")
plan.git.checkout(repo=cloned, ref="main")
plan.git.pull(repo=cloned)
t.expect_node_count(3)
