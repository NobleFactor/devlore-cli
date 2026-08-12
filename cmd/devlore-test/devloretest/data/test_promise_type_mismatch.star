# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_promise_type_mismatch.star — Verify the plan-time promise type-check fires end-to-end (phase-8 step 16).
#
# plan.file.mkdir produces a *file.Resource; binding its promise to write_text's os.FileMode chmod slot has no
# conversion path, so plan.assemble_definition fails with checkPromiseTypes' "cannot bind" violation.
# Plan-time validation only: the script never calls t.run.

t.expect_error("cannot bind")

made = plan.file.mkdir(path=t.tmp("made"), chmod=0o755)
bad  = plan.file.write_text(destination_path=t.tmp("out.txt"), content="x", chmod=made)

plan.assemble_definition([made, bad])
