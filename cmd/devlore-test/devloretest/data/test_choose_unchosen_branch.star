# test_choose_unchosen_branch.star — the step-10 goal proof: a side-effecting when or then on an unchosen or
# after-the-match branch must not execute.
#
# Under the value-picker this was unpassable — evaluating a when WAS running it, so every branch's side effects fired
# regardless of selection. Under the decision tree the short-circuit is structural: only the root-to-leaf path is
# dispatched, so the canary writes below must not exist after the run.
#
# Topology: case 1's when-body writes its canary and returns falsy (write_text's result feeds a falsy predicate via
# file.exists on a missing path — simpler: the when-body's last invocation is file.exists on a missing path, so the
# body runs its canary write first, then evaluates falsy). Case 2 matches. Case 3 and the default sit after the match:
# neither their whens nor their thens may run.

case1_when_canary = t.tmp("case1_when_ran.txt")      # allowed: case 1's when IS on the path (it must run to be found falsy)
case1_then_canary = t.tmp("case1_then_ran.txt")      # forbidden: case 1's then is unchosen (its when was falsy)
case2_then_result = t.tmp("case2_then_ran.txt")      # required: case 2 is the match
case3_when_canary = t.tmp("case3_when_ran.txt")      # forbidden: case 3's when sits after the match
case3_then_canary = t.tmp("case3_then_ran.txt")      # forbidden: case 3's then sits after the match
default_canary    = t.tmp("default_ran.txt")         # forbidden: the default sits after the match
status            = t.tmp("status.txt")

choice = plan.choose(
    plan.case(
        when=[
            plan.file.write_text(destination_path=case1_when_canary, content="ran", chmod=0o644),
            plan.file.exists(resource=t.tmp("never_created.txt")),
        ],
        then=plan.file.write_text(destination_path=case1_then_canary, content="must-not-run", chmod=0o644),
    ),
    plan.case(
        when=lambda: True,
        then=plan.file.write_text(destination_path=case2_then_result, content="chosen", chmod=0o644),
    ),
    plan.case(
        when=plan.file.write_text(destination_path=case3_when_canary, content="must-not-run", chmod=0o644),
        then=plan.file.write_text(destination_path=case3_then_canary, content="must-not-run", chmod=0o644),
    ),
    default=plan.file.write_text(destination_path=default_canary, content="must-not-run", chmod=0o644),
)
status_inv = plan.file.write_text(destination_path=status, content="done", chmod=0o644)

graph = plan.assemble_definition([choice, status_inv])

t.expect_file(case1_when_canary, content="ran")       # the falsy when on the path ran
t.expect_no_file(case1_then_canary)                   # unchosen then did not
t.expect_file(case2_then_result, content="chosen")    # the matched then ran
t.expect_no_file(case3_when_canary)                   # after-the-match when did not run
t.expect_no_file(case3_then_canary)                   # after-the-match then did not run
t.expect_no_file(default_canary)                      # the default did not run
t.expect_file(status, content="done")

t.run(graph)
