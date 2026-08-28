# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_choose_then_action.star — then= and default= carry ACTIONS, not just value lambdas.
#
# Every other choose fixture passes value-returning lambdas to then= and default= (lambda: "found"), so the
# action-bodied shape was uncovered. A package that must install software only when a predicate holds needs the
# branch to perform work rather than compute a string, and this proves it can.
#
# Deliberately lambda-free. A lambda is archived as a content-addressed function.Resource at plan time, and any
# graph carrying one currently fails receipt writing — see the choose fixtures that do use lambdas, which pass
# their expectations and then exit 1. Building the decision tree entirely from invocations avoids that, and is
# the shape package phase scripts should use.

dest    = t.tmp("choose_target.txt")
marker  = t.tmp("choose_marker.txt")
fallout = t.tmp("choose_default.txt")

written = plan.file.write_text(destination_path=dest, content="here", mode=0o644)
exists  = plan.file.exists(path=dest)

choice = plan.choose(
    plan.case(when=exists, then=plan.file.write_text(destination_path=marker, content="then-fired", mode=0o644)),
    default=plan.file.write_text(destination_path=fallout, content="default-fired", mode=0o644),
)

graph = plan.assemble_definition([written, choice])

# The predicate holds, so the then-body runs and the default-body is never dispatched.
t.expect_file(marker, content="then-fired")
t.expect_no_file(fallout)

t.run(graph)
