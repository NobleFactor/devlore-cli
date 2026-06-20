# test_choose_literals.star — table-style coverage of plan.choose with literal When values.
#
# Variations (Go-test-style table, one row per scenario):
#
#   1.  Single case, when=True            → returns case's Then
#   2.  Single case, when=False           → returns default
#   3.  Single case, when=1               → returns case's Then  (non-zero int truthy)
#   4.  Single case, when=0               → returns default      (zero falsy)
#   5.  Single case, when="x"             → returns case's Then  (non-empty string truthy)
#   6.  Single case, when=""              → returns default      (empty string falsy)
#   7.  Single case, when=None            → returns default      (None falsy)
#   8.  Zero cases                        → returns default
#   9.  Many cases, only second truthy    → returns second's Then (first-match-wins, but only one matches)
#  10.  Many cases, all truthy            → returns first's Then  (first-match-wins, multiple match)
#  11.  Many cases, none truthy           → returns default
#
# Each scenario routes its choice through write_text to a distinct status file; the t.expect_file
# assertions at the bottom verify per-scenario.

c_bool_true   = plan.choose("default", plan.case(when=True,  then="then-true"))
c_bool_false  = plan.choose("default", plan.case(when=False, then="then-false"))
c_int_one     = plan.choose("default", plan.case(when=1,     then="then-one"))
c_int_zero    = plan.choose("default", plan.case(when=0,     then="then-zero"))
c_str_x       = plan.choose("default", plan.case(when="x",   then="then-x"))
c_str_empty   = plan.choose("default", plan.case(when="",    then="then-empty"))
c_none        = plan.choose("default", plan.case(when=None,  then="then-none"))
c_zero_cases  = plan.choose("default")

c_second = plan.choose(
    "default",
    plan.case(when=False, then="then-1"),
    plan.case(when=True,  then="then-2"),
    plan.case(when=True,  then="then-3-not-fired"),
)

c_first_match = plan.choose(
    "default",
    plan.case(when=True, then="then-a"),
    plan.case(when=True, then="then-b"),
    plan.case(when=True, then="then-c"),
)

c_no_match = plan.choose(
    "default",
    plan.case(when=False, then="x"),
    plan.case(when=0,     then="y"),
    plan.case(when="",    then="z"),
)

w_bool_true   = plan.file.write_text(destination_path=t.tmp("bool_true.txt"),   content=c_bool_true,   chmod=0o644)
w_bool_false  = plan.file.write_text(destination_path=t.tmp("bool_false.txt"),  content=c_bool_false,  chmod=0o644)
w_int_one     = plan.file.write_text(destination_path=t.tmp("int_one.txt"),     content=c_int_one,     chmod=0o644)
w_int_zero    = plan.file.write_text(destination_path=t.tmp("int_zero.txt"),    content=c_int_zero,    chmod=0o644)
w_str_x       = plan.file.write_text(destination_path=t.tmp("str_x.txt"),       content=c_str_x,       chmod=0o644)
w_str_empty   = plan.file.write_text(destination_path=t.tmp("str_empty.txt"),   content=c_str_empty,   chmod=0o644)
w_none        = plan.file.write_text(destination_path=t.tmp("none.txt"),        content=c_none,        chmod=0o644)
w_zero_cases  = plan.file.write_text(destination_path=t.tmp("zero_cases.txt"),  content=c_zero_cases,  chmod=0o644)
w_second      = plan.file.write_text(destination_path=t.tmp("second.txt"),      content=c_second,      chmod=0o644)
w_first_match = plan.file.write_text(destination_path=t.tmp("first_match.txt"), content=c_first_match, chmod=0o644)
w_no_match    = plan.file.write_text(destination_path=t.tmp("no_match.txt"),    content=c_no_match,    chmod=0o644)

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
t.expect_unit_count(22)  # 11 choose + 11 write_text

t.run(graph)
