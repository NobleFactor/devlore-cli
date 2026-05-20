# test_net.star — Dry-run: appnet.download creates a graph node.
#
# Validates: plan.appnet.download (registration + node creation)

plan.appnet.download(url="https://example.com/file.txt")
t.expect_unit_count(1)
