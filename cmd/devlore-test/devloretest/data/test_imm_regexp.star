# test_imm_regexp.star — Immediate regexp operations.
#
# Validates: regexp.match, regexp.find, regexp.find_all, regexp.find_submatch,
#            regexp.find_all_submatch, regexp.replace, regexp.replace_literal, regexp.split

t.expect_equal(regexp.match(pattern="^hello", text="hello world"), True)
t.expect_equal(regexp.match(pattern="^world", text="hello world"), False)

t.expect_equal(regexp.find(pattern="[0-9]+", text="abc123def"), "123")

found = regexp.find_all(pattern="[0-9]+", text="a1b2c3", count=-1)
t.expect_equal(len(found), 3)

sub = regexp.find_submatch(pattern="([a-z]+)([0-9]+)", text="abc123")
t.expect_equal(sub[0], "abc123")
t.expect_equal(sub[1], "abc")
t.expect_equal(sub[2], "123")

all_sub = regexp.find_all_submatch(pattern="([0-9]+)", text="a1b2c3", count=-1)
t.expect_equal(len(all_sub), 3)

replaced = regexp.replace(pattern="[0-9]+", text="a1b2c3", replacement="X")
t.expect_equal(replaced, "aXbXcX")

# replace_literal does not interpret $ expansions in replacement
lit = regexp.replace_literal(pattern="[0-9]+", text="a1b2", replacement="$1")
t.expect_equal(lit, "a$1b$1")

parts = regexp.split(pattern=",", text="a,b,c", count=-1)
t.expect_equal(len(parts), 3)
t.expect_equal(parts[0], "a")

t.expect_unit_count(0)
