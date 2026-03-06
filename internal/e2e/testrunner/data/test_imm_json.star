# test_imm_json.star — Immediate JSON encode/decode.
#
# Validates: json.encode, json.decode, json.encode_indent

encoded = json.encode(value={"key": "value"})
t.expect_equal(encoded, '{"key":"value"}')

decoded = json.decode(data='{"a":1}')
t.expect_equal(decoded["a"], 1)

indented = json.encode_indent(value={"b": 2}, indent="  ")
t.expect_equal(json.decode(data=indented)["b"], 2)

t.expect_node_count(0)
