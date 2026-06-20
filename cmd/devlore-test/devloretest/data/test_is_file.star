# test_is_file.star — plan.choose dispatch driven by plan.file.is_file predicate.
#
# Write a file, check is_file (truthy), and use plan.choose to pick between the case's
# Then (`"is_file"`) and the default (`"not_file"`). Capture the chosen string in a
# downstream write_text to assert the value flowed through.

src    = t.tmp("is_file_src.txt")
status = t.tmp("is_file_status.txt")

written    = plan.file.write_text(destination_path=src, content="file check", chmod=0o644)
file_check = plan.file.is_file(resource=src)
choice     = plan.choose("not_file", plan.case(when=file_check, then="is_file"))
status_inv = plan.file.write_text(destination_path=status, content=choice, chmod=0o644)

graph = plan.assemble_definition([written, file_check, choice, status_inv])

t.expect_file(status, content="is_file")
t.expect_unit_count(4)  # write_text + is_file + choose + status_write

t.run(graph)
