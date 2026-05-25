# test_choose_predicates.star — plan.choose with predicates resolved from planned-method invocations.
#
# Each scenario uses a real-world predicate (plan.file.exists / is_dir / is_file) as the When value.
# Promises from those invocations are resolved at dispatch by flow.Choose's resolveDispatchedValue lookup
# in the runtime environment's Results map.
#
# Variations:
#
#   1. plan.file.exists on present file → case fires
#   2. plan.file.exists on missing file → default fires
#   3. plan.file.is_dir on existing dir → case fires
#   4. plan.file.is_file on existing file → case fires
#   5. Many predicate cases, only one truthy → that case's Then fires

present_path = t.tmp("present.txt")
missing_path = t.tmp("missing.txt")
dir_path     = t.tmp("a_directory")
file_path    = t.tmp("a_file.txt")

write_present = plan.file.write_text(destination_path=present_path, content="here", chmod=0o644)
make_dir      = plan.file.mkdir(path=dir_path, chmod=0o755)
write_file    = plan.file.write_text(destination_path=file_path, content="x", chmod=0o644)

exists_present = plan.file.exists(resource=present_path)
exists_missing = plan.file.exists(resource=missing_path)
is_dir_true    = plan.file.is_dir(resource=dir_path)
is_file_true   = plan.file.is_file(resource=file_path)

c_exists_present = plan.choose("default", plan.case(when=exists_present, then="exists-present"))
c_exists_missing = plan.choose("default", plan.case(when=exists_missing, then="exists-missing"))
c_is_dir         = plan.choose("default", plan.case(when=is_dir_true,    then="is-dir"))
c_is_file        = plan.choose("default", plan.case(when=is_file_true,   then="is-file"))

# Mixed multi-case: only the is_dir predicate is truthy.
c_mixed = plan.choose(
    "default",
    plan.case(when=exists_missing, then="missing-fired"),
    plan.case(when=is_dir_true,    then="mixed-is-dir-fired"),
    plan.case(when=is_file_true,   then="mixed-is-file-not-fired"),
)

w_exists_present = plan.file.write_text(destination_path=t.tmp("exists_present.txt"), content=c_exists_present, chmod=0o644)
w_exists_missing = plan.file.write_text(destination_path=t.tmp("exists_missing.txt"), content=c_exists_missing, chmod=0o644)
w_is_dir         = plan.file.write_text(destination_path=t.tmp("is_dir.txt"),         content=c_is_dir,         chmod=0o644)
w_is_file        = plan.file.write_text(destination_path=t.tmp("is_file.txt"),        content=c_is_file,        chmod=0o644)
w_mixed          = plan.file.write_text(destination_path=t.tmp("mixed.txt"),          content=c_mixed,          chmod=0o644)

graph = plan.assemble([
    write_present, make_dir, write_file,
    exists_present, exists_missing, is_dir_true, is_file_true,
    c_exists_present, c_exists_missing, c_is_dir, c_is_file, c_mixed,
    w_exists_present, w_exists_missing, w_is_dir, w_is_file, w_mixed,
])

t.expect_file(t.tmp("exists_present.txt"), content="exists-present")
t.expect_file(t.tmp("exists_missing.txt"), content="default")
t.expect_file(t.tmp("is_dir.txt"),         content="is-dir")
t.expect_file(t.tmp("is_file.txt"),        content="is-file")
t.expect_file(t.tmp("mixed.txt"),          content="mixed-is-dir-fired")
t.expect_unit_count(17)  # 3 setup + 4 predicates + 5 chooses + 5 writes

t.run(graph)
