# test_writ_adopt.star — Variable binding model: writ adopt happy path.
#
# Models the writ adopt workflow's mkdir → move → link sequence via plan.variable(...) declarations. Parameters
# are supplied via t.set_flags so the resolver picks them up under NamespaceFlag.
#
# 13.0(n) Phase 1: contract documentation only — Phase 2 (real resolver) populates variables; Phase 4
# (preflight validation) catches missing/mismatched values before dispatch; Phase 4 also wires the Go entry
# point that exercises this .star end-to-end via the harness.

src_dir   = t.tmp("adopt-src")
dest_dir  = t.tmp("adopt-dest")
src_path  = t.tmp("adopt-src/file.txt")
dest_path = t.tmp("adopt-dest/file.txt")

t.mkdir(src_dir)
t.write(src_path, "adopted content")

t.set_flags({
    "dest_dir":    dest_dir,
    "source_path": src_path,
    "dest_path":   dest_path,
})

plan.file.mkdir(path=plan.variable("dest_dir"), chmod=0o755)
plan.file.move(source=plan.variable("source_path"), destination_path=plan.variable("dest_path"))
plan.file.link(source=plan.variable("dest_path"), target_path=plan.variable("source_path"))

# Phase 4+ assertions:
#   t.expect_file(dest_path, content="adopted content")
#   t.expect_no_file(src_path)
#   t.expect_variable_namespace("dest_dir", "flag")
#   t.expect_variable_namespace("source_path", "flag")
#   t.expect_variable_namespace("dest_path", "flag")
