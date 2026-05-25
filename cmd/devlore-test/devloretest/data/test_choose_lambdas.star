# test_choose_lambdas.star — table-style coverage of plan.choose with lambda predicates.
#
# Lambdas pass through plan.case(when=...) into Case.When as *starlark.Function values; flow.Choose's
# resolveDispatchedValue invokes them at dispatch time and uses the unwrapped Go value for truthiness
# (When) and identity (Then).
#
# Variations:
#
#   1. when=lambda: True            → case fires
#   2. when=lambda: False           → default fires
#   3. when=lambda: 1               → case fires (non-zero int truthy)
#   4. when=lambda: 0               → default fires (zero falsy)
#   5. when=lambda: "x"             → case fires (non-empty string truthy)
#   6. when=lambda: ""              → default fires (empty string falsy)
#   7. when=lambda: None            → default fires
#   8. Many lambda cases, second matches → second's Then fires (lambda short-circuit applies — third's
#      lambda is never invoked because Choose breaks after the first truthy When)
#   9. then=lambda: "computed"      → the lambda's computed value flows through to the consumer

c_lambda_true     = plan.choose("default", plan.case(when=lambda: True,   then="lambda-true"))
c_lambda_false    = plan.choose("default", plan.case(when=lambda: False,  then="lambda-false"))
c_lambda_one      = plan.choose("default", plan.case(when=lambda: 1,      then="lambda-one"))
c_lambda_zero     = plan.choose("default", plan.case(when=lambda: 0,      then="lambda-zero"))
c_lambda_str_x    = plan.choose("default", plan.case(when=lambda: "x",    then="lambda-str-x"))
c_lambda_str_emp  = plan.choose("default", plan.case(when=lambda: "",     then="lambda-str-empty"))
c_lambda_none     = plan.choose("default", plan.case(when=lambda: None,   then="lambda-none"))

c_lambda_second = plan.choose(
    "default",
    plan.case(when=lambda: False, then="lambda-first"),
    plan.case(when=lambda: True,  then="lambda-second"),
    plan.case(when=lambda: True,  then="lambda-third-not-fired"),
)

c_lambda_then = plan.choose(
    "default",
    plan.case(when=True, then=lambda: "lambda-then-computed"),
)

w_lambda_true     = plan.file.write_text(destination_path=t.tmp("lambda_true.txt"),     content=c_lambda_true,    chmod=0o644)
w_lambda_false    = plan.file.write_text(destination_path=t.tmp("lambda_false.txt"),    content=c_lambda_false,   chmod=0o644)
w_lambda_one      = plan.file.write_text(destination_path=t.tmp("lambda_one.txt"),      content=c_lambda_one,     chmod=0o644)
w_lambda_zero     = plan.file.write_text(destination_path=t.tmp("lambda_zero.txt"),     content=c_lambda_zero,    chmod=0o644)
w_lambda_str_x    = plan.file.write_text(destination_path=t.tmp("lambda_str_x.txt"),    content=c_lambda_str_x,   chmod=0o644)
w_lambda_str_emp  = plan.file.write_text(destination_path=t.tmp("lambda_str_empty.txt"),content=c_lambda_str_emp, chmod=0o644)
w_lambda_none     = plan.file.write_text(destination_path=t.tmp("lambda_none.txt"),     content=c_lambda_none,    chmod=0o644)
w_lambda_second   = plan.file.write_text(destination_path=t.tmp("lambda_second.txt"),   content=c_lambda_second,  chmod=0o644)
w_lambda_then     = plan.file.write_text(destination_path=t.tmp("lambda_then.txt"),     content=c_lambda_then,    chmod=0o644)

graph = plan.assemble([
    c_lambda_true, c_lambda_false, c_lambda_one, c_lambda_zero,
    c_lambda_str_x, c_lambda_str_emp, c_lambda_none, c_lambda_second, c_lambda_then,
    w_lambda_true, w_lambda_false, w_lambda_one, w_lambda_zero,
    w_lambda_str_x, w_lambda_str_emp, w_lambda_none, w_lambda_second, w_lambda_then,
])

t.expect_file(t.tmp("lambda_true.txt"),      content="lambda-true")
t.expect_file(t.tmp("lambda_false.txt"),     content="default")
t.expect_file(t.tmp("lambda_one.txt"),       content="lambda-one")
t.expect_file(t.tmp("lambda_zero.txt"),      content="default")
t.expect_file(t.tmp("lambda_str_x.txt"),     content="lambda-str-x")
t.expect_file(t.tmp("lambda_str_empty.txt"), content="default")
t.expect_file(t.tmp("lambda_none.txt"),      content="default")
t.expect_file(t.tmp("lambda_second.txt"),    content="lambda-second")
t.expect_file(t.tmp("lambda_then.txt"),      content="lambda-then-computed")
t.expect_unit_count(18)  # 9 choose + 9 write_text

t.run(graph)
