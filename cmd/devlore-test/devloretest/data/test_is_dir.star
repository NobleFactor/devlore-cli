# test_is_dir.star — plan.choose dispatch driven by plan.file.is_dir predicate.
#
# Create a directory, check is_dir (truthy), and use plan.choose to pick between the
# case's Then (`"is_dir"`) and the default (`"not_dir"`). Capture the chosen string in
# a downstream write_text to assert the value flowed through.

dir    = t.tmp("is_dir_test")
status = t.tmp("is_dir_status.txt")

mkdir_inv  = plan.file.mkdir(path=dir, chmod=0o755)
dir_check  = plan.file.is_dir(resource=dir)
choice     = plan.choose("not_dir", plan.case(when=dir_check, then="is_dir"))
status_inv = plan.file.write_text(destination_path=status, content=choice, chmod=0o644)

graph = plan.assemble([mkdir_inv, dir_check, choice, status_inv])

t.expect_file(status, content="is_dir")
t.expect_unit_count(4)  # mkdir + is_dir + choose + status_write
