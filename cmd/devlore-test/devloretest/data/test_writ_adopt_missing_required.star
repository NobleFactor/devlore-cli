# test_writ_adopt_missing_required.star — Variable binding: missing required parameter.
#
# Declares a plan.var() with no default and no source value. Phase 4 (preflight validation) should aggregate
# the missing required into the D5 envelope and report a discoverable error before any node dispatches.
#
# 13.0(n) Phase 1: the .star file exists on disk as a contract artifact. No Go test entry point yet — the
# entry point lands once preflight (Phase 4) can produce the expected error cleanly. Until then this file
# documents the intended assertion form.

dest_dir = t.tmp("adopt-dest-missing")

# No t.set_flags / t.set_overrides — "dest_dir" is unresolvable.

plan.file.mkdir(path=plan.variable("dest_dir"), chmod=0o755)

# Phase 4+ assertion (currently inert because no Go entry point invokes this script):
#   t.expect_error("missing required parameter.*dest_dir")
