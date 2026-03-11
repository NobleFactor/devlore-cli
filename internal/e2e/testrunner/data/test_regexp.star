# test_regexp.star — Dry-run: regexp planned actions create graph nodes.
#
# Validates: plan.regexp.match, plan.regexp.find, plan.regexp.find_all,
#            plan.regexp.find_submatch, plan.regexp.find_all_submatch,
#            plan.regexp.replace, plan.regexp.replace_literal, plan.regexp.split

plan.regexp.match(pattern="foo", text="foobar")
plan.regexp.find(pattern="foo", text="foobar")
plan.regexp.find_all(pattern="o", text="foobar", count=-1)
plan.regexp.find_submatch(pattern="f(o+)", text="foobar")
plan.regexp.find_all_submatch(pattern="o", text="foobar", count=-1)
plan.regexp.replace(pattern="foo", text="foobar", replacement="baz")
plan.regexp.replace_literal(pattern="foo", text="foobar", replacement="baz")
plan.regexp.split(pattern=",", text="a,b,c", count=-1)
t.expect_node_count(8)
