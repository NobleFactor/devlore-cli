# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_orphan_unattached.star — Verify orphan detection at plan-end (phase-8 step 14).
#
# A constructed-but-unattached invocation must fail plan.assemble_definition with the orphan error.
# Plan-time validation only: the script never calls t.run.

t.expect_error("orphan invocation")

attached = plan.file.mkdir(path=t.tmp("made"), mode=0o755)
orphan   = plan.file.mkdir(path=t.tmp("stray"), mode=0o755)

# `orphan` is deliberately absent from the root set — never rooted by any container.
plan.assemble_definition([attached])
