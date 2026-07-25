# test_archive.star — Dry-run: archive.extract creates a graph node.
#
# Validates: plan.archive.extract (registration + node creation)

graph = plan.assemble_definition([
    plan.archive.extract(source="/tmp/fake.tar.gz", prefix_path=""),
])
t.expect_unit_count(1)
