# test_encryption.star — Dry-run: encryption.decrypt_sops_file creates a graph node.
#
# Validates: plan.encryption.decrypt_sops_file (registration + node creation)

plan.encryption.decrypt_sops_file(source="/tmp/fake.enc", destination="/tmp/fake.dec")
t.expect_node_count(1)
