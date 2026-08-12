# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_yaml.star — Dry-run: yaml planned actions create graph nodes.
#
# Validates: plan.yaml.encode, plan.yaml.decode

graph = plan.assemble_definition([
    plan.yaml.encode(value={"key": "value"}),
    plan.yaml.decode(data="key: value\n"),
])
t.expect_unit_count(2)
