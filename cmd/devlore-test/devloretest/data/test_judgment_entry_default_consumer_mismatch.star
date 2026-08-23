# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_entry_default_consumer_mismatch.star — judgment scenario: the entry default is permissive
# (docs/plans/resource-construction.md, explicit-conversion suite item 6).
#
# discover(path) with the `entry` default succeeds on whatever the disk holds — here a directory — and
# the failure surfaces at the CONSUMER's conversion when the discovered kind cannot fill the consumer's
# slot. This is the knowingly-carried cost of the enum design, pinned as designed behavior: assertion is
# where the verdict sharpens (item 5); the default is where it defers to the consumer.

dir = t.tmp("a-directory")
dst = t.tmp("never.txt")

t.mkdir(dir)

found = plan.file.discover(path=dir)
copied = plan.file.copy(source=found, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([found, copied])

t.expect_error("cannot fill")
t.expect_no_file(dst)

t.run(graph)
