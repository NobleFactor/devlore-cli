# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_kind_honest_activation.star — judgment scenario: claims are true when made (docs/plans/
# resource-construction.md, explicit-conversion suite item 12).
#
# A `*Regular` claim over a path whose disk entry is a SYMBOLIC LINK fails pre-flight with the kind
# verdict — kinds are lstat-strict, and kind-honest activation judges the claimed kind at the starting
# line instead of activating kind-blind and failing later (or worse, silently reading through the link).
# The link is OUT-OF-BAND (t.symlink — the harness's own disk write, no provider, no catalog): the model
# never sees it coming, which is precisely door four of the false-claim taxonomy, met at the starting
# line. The directory direction rides the Go pins.

target = t.tmp("real.txt")
link = t.tmp("the-link")
dst = t.tmp("never.txt")

t.write(target, "the link's target")
t.symlink(target, link)

copied = plan.file.copy(source=link, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([copied])

t.expect_error("verify existence")
t.expect_no_file(dst)

t.run(graph)
