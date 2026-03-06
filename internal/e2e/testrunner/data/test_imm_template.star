# test_imm_template.star — Immediate template rendering.
#
# Validates: template.render (immediate mode)
# content param is []byte, use b"..." in Starlark.

result = template.render(
    template_data={"Name": "world"},
    source="",
    path="",
    project="test",
    content=b"hello {{.Name}}",
)
t.expect_equal(result, b"hello world")

t.expect_node_count(0)
