# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_encryption.star — Dry-run: encryption nodes project through plan.encryption.
#
# Validates: plan.encryption.decrypt_sops_file and plan.encryption.encrypt_file
# (registration + node creation)

graph = plan.assemble_definition([
    plan.encryption.decrypt_sops_file(source="/tmp/fake.enc", destination_path="/tmp/fake.dec"),
    plan.encryption.encrypt_file(source="/tmp/fake.dec", destination_path="/tmp/fake.enc"),
])
t.expect_unit_count(2)
