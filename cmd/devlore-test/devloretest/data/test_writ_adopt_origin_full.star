# test_writ_adopt_origin_full.star — Variable binding: full origin string assertion.
#
# Supplies a parameter via env (program-specific prefix) so the resolver records the literal env var name in
# Origin.Name. Asserts the full "<namespace>:<name>" form via t.expect_variable.
#
# 13.0(n) Phase 1: contract documentation only — Phase 2 (real resolver) reads env; Phase 4 wires the Go
# entry point.

dest_dir = t.tmp("adopt-dest-origin-full")

t.set_env_prefix("DEVLORE_TEST")
# The Go-side test harness would do `os.Setenv("DEVLORE_TEST_DEST_DIR", dest_dir)` before invoking the
# runner; for Phase 1 this stays as documentation.

plan.file.mkdir(path=plan.variable("dest_dir"), chmod=0o755)

# Phase 4+ assertion:
#   t.expect_variable("dest_dir", origin="env:DEVLORE_TEST_DEST_DIR")
