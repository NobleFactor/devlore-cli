# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_template_render_bytes.star — Render a Go template to bytes via planned action.
#
# Validates: plan.template.render_bytes

graph = plan.assemble_definition([
    plan.template.render_bytes(
        content="hello {{.Name}}",
        data={"Name": "world"},
    ),
])
t.expect_unit_count(1)

t.run(graph)
