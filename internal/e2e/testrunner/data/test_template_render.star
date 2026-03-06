# test_template_render.star — Render a Go template via planned action.
#
# Validates: plan.template.render

plan.template.render(
    template_data={"Name": "world"},
    source="",
    path="",
    project="test",
    content="hello {{.Name}}",
)
t.expect_node_count(1)
