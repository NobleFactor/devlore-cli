# test_regex.star — Dry-run: regex planned actions create graph nodes.
#
# Validates: plan.regex.match, plan.regex.find, plan.regex.find_all,
#            plan.regex.find_submatch, plan.regex.find_all_submatch,
#            plan.regex.replace, plan.regex.replace_literal, plan.regex.split

graph = plan.assemble_definition([
    plan.regex.match(pattern="foo", text="foobar"),
    plan.regex.find(pattern="foo", text="foobar"),
    plan.regex.find_all(pattern="o", text="foobar", count=-1),
    plan.regex.find_submatch(pattern="f(o+)", text="foobar"),
    plan.regex.find_all_submatch(pattern="o", text="foobar", count=-1),
    plan.regex.replace(pattern="foo", text="foobar", replacement="baz"),
    plan.regex.replace_literal(pattern="foo", text="foobar", replacement="baz"),
    plan.regex.split(pattern=",", text="a,b,c", count=-1),
])
t.expect_unit_count(8)
