# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_move_missing_source.star — judgment scenario: a missing move source is unmet intent
# (docs/plans/any-entry-claims.md, phase 4).
#
# `file.move` has no tolerance parameter: moving something that is gone accomplishes nothing, and a
# tolerated miss would hand downstream consumers a nil product — the pathology that had `Skip` dropped
# from MissingResourcePolicy (#605). So the claim is required, and the verdict lands at PRE-FLIGHT, before
# any unit dispatches — not as a mid-run I/O error rediscovered by the mover.
#
# The sharp assertion is the error's shape: the catalog's "verify existence" verdict, not a rename failure.

source = t.tmp("vanishes.txt")
destination = t.tmp("never.txt")

t.write(source, "gone before the run")

moved = plan.file.move(source=source, destination_path=destination)
graph = plan.assemble_definition([moved])

# Intent breaks between planning and the run.
file.remove(target=source, prune=False, boundary="")

t.expect_error("verify existence")
t.expect_no_file(destination)

t.run(graph)
