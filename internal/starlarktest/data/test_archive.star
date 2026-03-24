# test_archive.star — Dry-run: archive.extract creates a graph node.
#
# Validates: plan.archive.extract (registration + node creation)

plan.archive.extract(source="/tmp/fake.tar.gz", prefix="")
t.expect_node_count(1)
