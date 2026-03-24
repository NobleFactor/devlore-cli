# test_yaml.star — Dry-run: yaml planned actions create graph nodes.
#
# Validates: plan.yaml.encode, plan.yaml.decode

plan.yaml.encode(value={"key": "value"})
plan.yaml.decode(data="key: value\n")
t.expect_node_count(2)
