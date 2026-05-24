# test_template_render.star — Render a Go template via planned action.
#
# Validates: plan.template.render_text

graph = plan.assemble([
    plan.template.render_text(
        content="hello {{.Name}}",
        data={"Name": "world"},
    ),
])
t.expect_unit_count(1)
