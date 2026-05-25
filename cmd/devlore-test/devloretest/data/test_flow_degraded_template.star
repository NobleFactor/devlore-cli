# test_flow_degraded_template.star — Verify degraded with template kwargs.
# The write_text result flows into the degraded template via a promise.
# Graph should still complete successfully.

written  = plan.file.write_text(destination_path=t.tmp("d.txt"), content="ok", chmod=0o644)
degraded = plan.degraded("wrote {{ .path }}", path=written)

graph = plan.assemble([written, degraded])

t.expect_unit_count(2)

t.run(graph)
