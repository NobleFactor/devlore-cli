# test_imm_yaml.star — Immediate YAML encode/decode.
#
# Validates: yaml.encode, yaml.decode

encoded = yaml.encode({"key": "value"})
decoded = yaml.decode(encoded)
t.expect_equal(decoded["key"], "value")

t.expect_unit_count(0)
