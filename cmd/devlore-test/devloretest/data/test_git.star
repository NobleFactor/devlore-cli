# test_git.star — Dry-run: git actions create graph nodes.
#
# Validates: plan.git.clone, plan.git.checkout, plan.git.pull (registration + node creation)

cloned       = plan.git.clone(repository="https://example.com/repo.git", directory="/tmp/repo")
checked_out  = plan.git.checkout(repo=cloned, ref="main")
pulled       = plan.git.pull(repo=cloned)

graph = plan.assemble([cloned, checked_out, pulled])
t.expect_unit_count(3)
