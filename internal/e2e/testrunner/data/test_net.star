# test_net.star — Dry-run: net.download creates a graph node.
#
# Validates: plan.net.download (registration + node creation)

plan.net.download(url="https://example.com/file.txt")
t.expect_node_count(1)
