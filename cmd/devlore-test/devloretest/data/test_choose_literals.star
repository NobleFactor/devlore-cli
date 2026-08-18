# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_choose_literals.star — table-style coverage of plan.choose truthiness routing over lambda bodies.
#
# Literal when-values from the value-picker era become zero-arg lambdas: a lambda body desugars to a function.call
# leaf (the lambda rides the graph as a content-addressed function.Resource), the when-subgraph's result is the
# lambda's value, and the decision tree routes on its truthiness.
#
# Variations (Go-test-style table, one row per scenario):
#
#   1.  Single case, when=lambda: True    → returns case's then
#   2.  Single case, when=lambda: False   → returns default
#   3.  Single case, when=lambda: 1       → returns case's then  (non-zero int truthy)
#   4.  Single case, when=lambda: 0       → returns default      (zero falsy)
#   5.  Single case, when=lambda: "x"     → returns case's then  (non-empty string truthy)
#   6.  Single case, when=lambda: ""      → returns default      (empty string falsy)
#   7.  Single case, when=lambda: None    → returns default      (None falsy)
#   8.  Zero cases                        → returns default      (degenerate form, defined behavior)
#   9.  Many cases, only second truthy    → returns second's then
#  10.  Many cases, all truthy            → returns first's then  (first-truthy short-circuit)
#  11.  Many cases, none truthy           → returns default
#
# Each scenario routes its choice through write_text to a distinct status file; the t.expect_file
# assertions at the bottom verify per-scenario.

c_bool_true  = plan.choose(plan.case(when=lambda: True,  then=lambda: "then-true"),  default=lambda: "default")
c_bool_false = plan.choose(plan.case(when=lambda: False, then=lambda: "then-false"), default=lambda: "default")
c_int_one    = plan.choose(plan.case(when=lambda: 1,     then=lambda: "then-one"),   default=lambda: "default")
c_int_zero   = plan.choose(plan.case(when=lambda: 0,     then=lambda: "then-zero"),  default=lambda: "default")
c_str_x      = plan.choose(plan.case(when=lambda: "x",   then=lambda: "then-x"),     default=lambda: "default")
c_str_empty  = plan.choose(plan.case(when=lambda: "",    then=lambda: "then-empty"), default=lambda: "default")
c_none       = plan.choose(plan.case(when=lambda: None,  then=lambda: "then-none"),  default=lambda: "default")
c_zero_cases = plan.choose(default=lambda: "default")

c_second = plan.choose(
    plan.case(when=lambda: False, then=lambda: "then-1"),
    plan.case(when=lambda: True,  then=lambda: "then-2"),
    plan.case(when=lambda: True,  then=lambda: "then-3-not-fired"),
    default=lambda: "default",
)

c_first_match = plan.choose(
    plan.case(when=lambda: True, then=lambda: "then-a"),
    plan.case(when=lambda: True, then=lambda: "then-b"),
    plan.case(when=lambda: True, then=lambda: "then-c"),
    default=lambda: "default",
)

c_no_match = plan.choose(
    plan.case(when=lambda: False, then=lambda: "x"),
    plan.case(when=lambda: 0,     then=lambda: "y"),
    plan.case(when=lambda: "",    then=lambda: "z"),
    default=lambda: "default",
)

w_bool_true   = plan.file.write_text(destination_path=t.tmp("bool_true.txt"),   content=c_bool_true,   mode=0o644)
w_bool_false  = plan.file.write_text(destination_path=t.tmp("bool_false.txt"),  content=c_bool_false,  mode=0o644)
w_int_one     = plan.file.write_text(destination_path=t.tmp("int_one.txt"),     content=c_int_one,     mode=0o644)
w_int_zero    = plan.file.write_text(destination_path=t.tmp("int_zero.txt"),    content=c_int_zero,    mode=0o644)
w_str_x       = plan.file.write_text(destination_path=t.tmp("str_x.txt"),       content=c_str_x,       mode=0o644)
w_str_empty   = plan.file.write_text(destination_path=t.tmp("str_empty.txt"),   content=c_str_empty,   mode=0o644)
w_none        = plan.file.write_text(destination_path=t.tmp("none.txt"),        content=c_none,        mode=0o644)
w_zero_cases  = plan.file.write_text(destination_path=t.tmp("zero_cases.txt"),  content=c_zero_cases,  mode=0o644)
w_second      = plan.file.write_text(destination_path=t.tmp("second.txt"),      content=c_second,      mode=0o644)
w_first_match = plan.file.write_text(destination_path=t.tmp("first_match.txt"), content=c_first_match, mode=0o644)
w_no_match    = plan.file.write_text(destination_path=t.tmp("no_match.txt"),    content=c_no_match,    mode=0o644)

graph = plan.assemble_definition([
    c_bool_true, c_bool_false, c_int_one, c_int_zero,
    c_str_x, c_str_empty, c_none, c_zero_cases,
    c_second, c_first_match, c_no_match,
    w_bool_true, w_bool_false, w_int_one, w_int_zero,
    w_str_x, w_str_empty, w_none, w_zero_cases,
    w_second, w_first_match, w_no_match,
])

t.expect_file(t.tmp("bool_true.txt"),   content="then-true")
t.expect_file(t.tmp("bool_false.txt"),  content="default")
t.expect_file(t.tmp("int_one.txt"),     content="then-one")
t.expect_file(t.tmp("int_zero.txt"),    content="default")
t.expect_file(t.tmp("str_x.txt"),       content="then-x")
t.expect_file(t.tmp("str_empty.txt"),   content="default")
t.expect_file(t.tmp("none.txt"),        content="default")
t.expect_file(t.tmp("zero_cases.txt"),  content="default")
t.expect_file(t.tmp("second.txt"),      content="then-2")
t.expect_file(t.tmp("first_match.txt"), content="then-a")
t.expect_file(t.tmp("no_match.txt"),    content="default")
# Units per single-case choose: 4 subgraphs (choose/when/then/default) + 3 function.call nodes = 7 (×7 = 49).
# Zero-case choose: choose + default subgraphs + 1 call node = 3. Three-case chooses: 1 + 3×4 + 2 = 15 (×3 = 45).
# Plus 11 write_text nodes.
t.expect_unit_count(108)

t.run(graph)
