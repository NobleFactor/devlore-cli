# test_mkdir_and_remove_all.star — Create a directory tree and remove it.
#
# 1. Create a directory with mkdir
# 2. Write a file inside it
# 3. Remove the entire tree with remove_all
#
# Validates: plan.file.mkdir, plan.file.write_text, plan.file.remove_all

dir  = t.tmp("mydir")
file = t.tmp("mydir/nested.txt")

graph = plan.assemble_definition([
    plan.file.mkdir(path=dir, chmod=0o755),
    plan.file.write_text(destination_path=file, content="nested content", chmod=0o644),
    plan.file.remove_all(resource=dir, prune=False, boundary=""),
])

t.expect_no_file(file)
t.expect_unit_count(3)

t.run(graph)
