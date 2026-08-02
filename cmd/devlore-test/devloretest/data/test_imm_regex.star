# test_imm_regex.star — Immediate regex operations.
#
# Validates: regex.match, regex.find, regex.find_all, regex.find_submatch,
#            regex.find_all_submatch, regex.replace, regex.replace_literal, regex.split

t.expect_equal(regex.match(pattern="^hello", text="hello world"), True)
t.expect_equal(regex.match(pattern="^world", text="hello world"), False)

t.expect_equal(regex.find(pattern="[0-9]+", text="abc123def"), "123")

found = regex.find_all(pattern="[0-9]+", text="a1b2c3", count=-1)
t.expect_equal(len(found), 3)

sub = regex.find_submatch(pattern="([a-z]+)([0-9]+)", text="abc123")
t.expect_equal(sub[0], "abc123")
t.expect_equal(sub[1], "abc")
t.expect_equal(sub[2], "123")

all_sub = regex.find_all_submatch(pattern="([0-9]+)", text="a1b2c3", count=-1)
t.expect_equal(len(all_sub), 3)

replaced = regex.replace(pattern="[0-9]+", text="a1b2c3", replacement="X")
t.expect_equal(replaced, "aXbXcX")

# replace_literal does not interpret $ expansions in replacement
lit = regex.replace_literal(pattern="[0-9]+", text="a1b2", replacement="$1")
t.expect_equal(lit, "a$1b$1")

parts = regex.split(pattern=",", text="a,b,c", count=-1)
t.expect_equal(len(parts), 3)
t.expect_equal(parts[0], "a")

t.expect_unit_count(0)
