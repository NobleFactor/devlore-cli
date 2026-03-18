# test_imm_template.star — Immediate template rendering.
#
# Validates: template.render_text (immediate mode)

result = template.render_text(
    content="hello {{.Name}}",
    data={"Name": "world"},
)
t.expect_equal(result, "hello world")

t.expect_node_count(0)
