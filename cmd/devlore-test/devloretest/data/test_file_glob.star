# test_file_glob.star — Create files and glob for them.
#
# Validates: plan.file.mkdir, plan.file.write_text, plan.file.glob

dir = t.tmp("globdir")
graph = plan.assemble_definition([
    plan.file.mkdir(path=dir, chmod=0o755),
    plan.file.write_text(destination_path=t.tmp("globdir/a.txt"), content="a", chmod=0o644),
    plan.file.write_text(destination_path=t.tmp("globdir/b.txt"), content="b", chmod=0o644),
    plan.file.glob(pattern=t.tmp("globdir/*.txt"), include_gitignored=True),
])

t.expect_file(t.tmp("globdir/a.txt"), content="a")
t.expect_file(t.tmp("globdir/b.txt"), content="b")
t.expect_unit_count(4)  # mkdir + write_text + write_text + glob

t.run(graph)
