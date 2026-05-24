# test_encryption.star — Dry-run: encryption.decrypt_sops_file creates a graph node.
#
# Validates: plan.encryption.decrypt_sops_file (registration + node creation)

graph = plan.assemble([
    plan.encryption.decrypt_sops_file(source="/tmp/fake.enc", destination_path="/tmp/fake.dec"),
])
t.expect_unit_count(1)
