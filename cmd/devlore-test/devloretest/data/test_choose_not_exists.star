# test_choose_not_exists.star — plan.choose with a false predicate (no then-branch).
#
# Check existence of a file that was never created.
# The predicate returns false, so the then-branch should NOT execute.
# The file we would have written in the then-branch should not exist.

phantom = t.tmp("phantom.txt")
canary  = t.tmp("canary.txt")

# Predicate: file does not exist.
exists_output = plan.file.exists(resource=phantom)

# Then-branch would create canary — but it should not fire.
plan.choose(
    when=exists_output,
    then=lambda: plan.file.write_text(destination_path=canary, content="should not exist", mode=0o644),
)

t.expect_no_file(phantom)
t.expect_no_file(canary)
t.expect_node_count(3)  # exists + write_text (in branch) + choose
