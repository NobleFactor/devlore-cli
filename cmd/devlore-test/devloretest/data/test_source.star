# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_source.star — Use plan.file.read_text to read an existing file.
#
# The file exists BEFORE the run (written by the harness, outside the graph), so the read's claim is honest
# required intent: pre-flight verifies it under the run's root and the read consumes it. The former shape —
# a shell unit creating the file for a later read by name, with no promise between them — is refused by
# scoped pre-flight, by design (§3: ordering-by-coincidence).
#
# Validates: plan.file.read_text

dest = t.tmp("source_input.txt")
t.write(dest, "source test")

graph = plan.assemble_definition([
    plan.file.read_text(resource=dest),
])

t.expect_file(dest, content="source test")
t.expect_unit_count(1)  # file.read_text

t.run(graph)
