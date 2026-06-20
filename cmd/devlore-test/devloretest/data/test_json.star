# test_json.star — Dry-run: json planned actions create graph nodes.
#
# Validates: plan.json.encode, plan.json.encode_indent, plan.json.decode

graph = plan.assemble_definition([
    plan.json.encode(value={"key": "value"}),
    plan.json.encode_indent(value={"key": "value"}, indent="  "),
    plan.json.decode(data='{"key":"value"}'),
])
t.expect_unit_count(3)
