# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_is_dir.star — plan.choose dispatch driven by a plan.file.is_dir when-body.
#
# Create a directory, check is_dir inside the case's when-body (truthy), and route to the then-body ("is_dir") over
# the default ("not_dir"). Capture the chosen string in a downstream write_text to assert the value flowed through.

dir    = t.tmp("is_dir_test")
status = t.tmp("is_dir_status.txt")

mkdir_inv  = plan.file.mkdir(path=dir, chmod=0o755)
dir_check  = plan.file.is_dir(path=dir)
choice     = plan.choose(plan.case(when=dir_check, then=lambda: "is_dir"), default=lambda: "not_dir")
status_inv = plan.file.write_text(destination_path=status, content=choice, chmod=0o644)

graph = plan.assemble_definition([mkdir_inv, choice, status_inv])

t.expect_file(status, content="is_dir")
t.expect_unit_count(9)  # nodes: mkdir + is_dir + then-call + default-call + status_write; subgraphs: choose + when + then + default

t.run(graph)
