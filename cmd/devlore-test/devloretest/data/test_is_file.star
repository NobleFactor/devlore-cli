# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_is_file.star — plan.choose dispatch driven by a plan.file.is_file when-body.
#
# Write a file, check is_file inside the case's when-body (truthy), and route to the then-body ("is_file") over the
# default ("not_file"). Capture the chosen string in a downstream write_text to assert the value flowed through.

src    = t.tmp("is_file_src.txt")
status = t.tmp("is_file_status.txt")

written    = plan.file.write_text(destination_path=src, content="file check", mode=0o644)
file_check = plan.file.is_file(path=src)
choice     = plan.choose(plan.case(when=file_check, then=lambda: "is_file"), default=lambda: "not_file")
status_inv = plan.file.write_text(destination_path=status, content=choice, mode=0o644)

graph = plan.assemble_definition([written, choice, status_inv])

t.expect_file(status, content="is_file")
t.expect_unit_count(9)  # nodes: written + is_file + then-call + default-call + status_write; subgraphs: choose + when + then + default

t.run(graph)
