# test_flow_fatal_template.star — Verify fatal with template kwargs.
# The write_text result flows into the fatal template via a promise.

t.expect_error("fatal:.*startup failed")

svc   = plan.file.write_text(destination_path=t.tmp("svc.txt"), content="myapp", chmod=0o644)
fatal = plan.fatal("{{ .service }} startup failed", service=svc)

graph = plan.assemble([svc, fatal])
