# test_choose_exists.star — plan.choose returns the matched case's Then value.
#
# Write a file, check its existence (truthy), and use plan.choose to pick between
# the case's Then (`"found"`) and the default (`"missing"`). Capture the chosen
# string in a downstream write_text so we can assert the value flowed through.

dest   = t.tmp("choose_target.txt")
status = t.tmp("choose_status.txt")

written    = plan.file.write_text(destination_path=dest, content="here", chmod=0o644)
exists_inv = plan.file.exists(resource=dest)
choice     = plan.choose("missing", plan.case(when=exists_inv, then="found"))
status_inv = plan.file.write_text(destination_path=status, content=choice, chmod=0o644)

graph = plan.assemble_definition([written, exists_inv, choice, status_inv])

t.expect_file(status, content="found")
t.expect_unit_count(4)  # write_text + exists + choose + status_write

t.run(graph)
