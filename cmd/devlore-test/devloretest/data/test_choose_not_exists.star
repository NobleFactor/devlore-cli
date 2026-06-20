# test_choose_not_exists.star — plan.choose returns the default when no case matches.
#
# Check existence of a file that was never created (predicate returns false). plan.choose
# falls through to the default value. Capture it in a downstream write_text to assert.

phantom = t.tmp("phantom.txt")
status  = t.tmp("choose_status.txt")

exists_inv = plan.file.exists(resource=phantom)
choice     = plan.choose("missing", plan.case(when=exists_inv, then="found"))
status_inv = plan.file.write_text(destination_path=status, content=choice, chmod=0o644)

graph = plan.assemble_definition([exists_inv, choice, status_inv])

t.expect_no_file(phantom)
t.expect_file(status, content="missing")
t.expect_unit_count(3)  # exists + choose + status_write

t.run(graph)
