# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_flow_fatal_recovery.star — Verify compensable actions before a terminal failure are unwound.
# write_text is compensable; after plan.failed halts the run, the file should be removed.
#
# The error text names the action that raised it: "flow.failed executed: <message>". It carried a
# "fatal:" prefix before the action was renamed from fatal to failed.

dest = t.tmp("to-be-undone.txt")

written = plan.file.write_text(destination_path=dest, content="temporary", mode=0o644)
fatal   = plan.failed("abort after write")

graph = plan.assemble_definition([written, fatal])

t.expect_error("flow.failed executed: abort after write")
t.expect_no_file(dest)

t.run(graph)
