# test_choose_predicates.star — plan.choose with real-world predicate when-bodies.
#
# Each scenario uses a planned predicate (plan.file.exists / is_dir / is_file) as the case's when-body — a singleton
# invocation adopted as the when-subgraph's child. The subgraph's result is the predicate's result; the decision tree
# routes on its truthiness. A predicate invocation is adopted by exactly one when-subgraph, so the mixed multi-case
# scenario mints fresh invocations rather than reusing the single-case ones.
#
# Variations:
#
#   1. plan.file.exists on present file → case fires
#   2. plan.file.exists on missing file → default fires
#   3. plan.file.is_dir on existing dir → case fires
#   4. plan.file.is_file on existing file → case fires
#   5. Many predicate cases, only one truthy → that case's then fires

present_path = t.tmp("present.txt")
missing_path = t.tmp("missing.txt")
dir_path     = t.tmp("a_directory")
file_path    = t.tmp("a_file.txt")

write_present = plan.file.write_text(destination_path=present_path, content="here", chmod=0o644)
make_dir      = plan.file.mkdir(path=dir_path, chmod=0o755)
write_file    = plan.file.write_text(destination_path=file_path, content="x", chmod=0o644)

c_exists_present = plan.choose(
    plan.case(when=plan.file.exists(path=present_path), then=lambda: "exists-present"),
    default=lambda: "default",
)
c_exists_missing = plan.choose(
    plan.case(when=plan.file.exists(path=missing_path), then=lambda: "exists-missing"),
    default=lambda: "default",
)
c_is_dir = plan.choose(
    plan.case(when=plan.file.is_dir(path=dir_path), then=lambda: "is-dir"),
    default=lambda: "default",
)
c_is_file = plan.choose(
    plan.case(when=plan.file.is_file(path=file_path), then=lambda: "is-file"),
    default=lambda: "default",
)

# Mixed multi-case: only the is_dir predicate is truthy. The third case's when-subgraph sits after the match and is
# never dispatched — the short-circuit is structural.
c_mixed = plan.choose(
    plan.case(when=plan.file.exists(path=missing_path), then=lambda: "missing-fired"),
    plan.case(when=plan.file.is_dir(path=dir_path),     then=lambda: "mixed-is-dir-fired"),
    plan.case(when=plan.file.is_file(path=file_path),   then=lambda: "mixed-is-file-not-fired"),
    default=lambda: "default",
)

w_exists_present = plan.file.write_text(destination_path=t.tmp("exists_present.txt"), content=c_exists_present, chmod=0o644)
w_exists_missing = plan.file.write_text(destination_path=t.tmp("exists_missing.txt"), content=c_exists_missing, chmod=0o644)
w_is_dir         = plan.file.write_text(destination_path=t.tmp("is_dir.txt"),         content=c_is_dir,         chmod=0o644)
w_is_file        = plan.file.write_text(destination_path=t.tmp("is_file.txt"),        content=c_is_file,        chmod=0o644)
w_mixed          = plan.file.write_text(destination_path=t.tmp("mixed.txt"),          content=c_mixed,          chmod=0o644)

graph = plan.assemble_definition([
    write_present, make_dir, write_file,
    c_exists_present, c_exists_missing, c_is_dir, c_is_file, c_mixed,
    w_exists_present, w_exists_missing, w_is_dir, w_is_file, w_mixed,
])

t.expect_file(t.tmp("exists_present.txt"), content="exists-present")
t.expect_file(t.tmp("exists_missing.txt"), content="default")
t.expect_file(t.tmp("is_dir.txt"),         content="is-dir")
t.expect_file(t.tmp("is_file.txt"),        content="is-file")
t.expect_file(t.tmp("mixed.txt"),          content="mixed-is-dir-fired")
# Units: 3 setup nodes; per single-predicate choose 4 subgraphs + predicate node + 2 call nodes = 7 (×4 = 28); the
# three-case mixed choose 1 + 3×(when-sg + predicate + then-sg + call) + default-sg + call = 15; 5 write_text nodes.
t.expect_unit_count(51)

t.run(graph)
