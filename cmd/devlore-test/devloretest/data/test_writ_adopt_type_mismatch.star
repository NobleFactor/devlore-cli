# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_writ_adopt_type_mismatch.star — Variable binding: type mismatch at preflight.
#
# Supplies an override of the wrong Go type for a parameter (mode expects an int mode; flag gives a string).
# Phase 4 (preflight validation) should detect the mismatch during the bindVariables pass and aggregate it
# into the D5 envelope.
#
# 13.0(n) Phase 1: contract documentation only — no Go entry point yet (lands when preflight produces the
# expected discoverable error in Phase 4).

t.set_flags({
    "mode": "not_an_int",  # plan.file.mkdir.mode is os.FileMode; string is the wrong type
})

graph = plan.assemble_definition([
    plan.file.mkdir(path=t.tmp("type-mismatch-dest"), mode=plan.variable("mode")),
])

t.expect_error("mode.*not assignable to declared type")

t.run(graph)
