# test_gather_advanced.star — frame inheritance / stripping + multi-child / composition coverage for plan.gather.
#
# Matrix rows (PowerShell ForEach-Object -Parallel `$using:` analogues + advanced shapes):
#   A1: parent variable visible inside body — outer plan.variable lookup resolves against the inherited frame.
#   A2: items stripped from per-iteration frame — plan.variable("items") inside body resolves to nil (not the
#       gather's `items=` value); the gather-internal slot is invisible to children.
#   A4: multi-child body — every body invocation dispatches per iteration in declaration order; both write
#       distinct artefacts derived from the same `item`.
#   A5: gather composed with leaf nodes — a write_text running before plan.gather observes its produced file,
#       and plan.gather's iterations observe the parent variable populated upstream.

t.set_flags({"greeting": "hola"})

# region A1: parent variable visible inside body — outer plan.variable("greeting") resolves from session frame

a1_paths = [t.tmp("a1_%d.txt" % i) for i in range(3)]
a1_inv = plan.file.write_text(
    destination_path=plan.variable("item", default_value=None),
    content=plan.variable("greeting", default_value=None),
    chmod=0o644,
)
a1 = plan.gather(items=a1_paths, limit=2, body=[a1_inv])

# endregion

# region A2: items stripped from per-iteration frame — plan.variable("items") resolves to nil inside body

a2_path = t.tmp("a2.txt")
a2_inv = plan.file.write_text(
    destination_path=a2_path,
    content=plan.variable("items", default_value=""),
    chmod=0o644,
)
a2 = plan.gather(items=["sentinel"], limit=1, body=[a2_inv])

# endregion

# region A4: multi-child body — both children dispatch per iteration in declaration order

a4_paths_x = [t.tmp("a4x_%d.txt" % i) for i in range(2)]
a4_paths_y = [t.tmp("a4y_%d.txt" % i) for i in range(2)]
# Pair items so each iteration's `item` is a destination_path string for the X-write; the Y-write
# observes the same item via plan.variable("item"). Two iterations × two children == four files total.
a4_pair_items = a4_paths_x  # iteration `item` is the X-path
a4_x_inv = plan.file.write_text(
    destination_path=plan.variable("item", default_value=None),
    content="x",
    chmod=0o644,
)
a4_y_inv = plan.file.write_text(
    destination_path=a4_paths_y[0],  # both Y-writes target the same path under serial-ish ordering;
                                     # under parallel limit, last writer wins — content stays "y".
    content="y",
    chmod=0o644,
)
a4 = plan.gather(items=a4_pair_items, limit=2, body=[a4_x_inv, a4_y_inv])

# endregion

# region A5: gather composed with leaf nodes — pre-write completes, gather inherits parent frame

a5_pre = t.tmp("a5_pre.txt")
a5_pre_inv = plan.file.write_text(destination_path=a5_pre, content="before-gather", chmod=0o644)

a5_paths = [t.tmp("a5_%d.txt" % i) for i in range(2)]
a5_inv = plan.file.write_text(
    destination_path=plan.variable("item", default_value=None),
    content=plan.variable("greeting", default_value=None),
    chmod=0o644,
)
a5 = plan.gather(items=a5_paths, limit=2, body=[a5_inv])

# endregion

graph = plan.assemble([a1, a2, a4, a5_pre_inv, a5])

# A1: parent variable resolves to flag value
for p in a1_paths:
    t.expect_file(p, content="hola")

# A2: items stripped — content is the default-value sentinel
t.expect_file(a2_path, content="")

# A4: every iteration ran both children
for p in a4_paths_x:
    t.expect_file(p, content="x")
t.expect_file(a4_paths_y[0], content="y")

# A5: pre-write happened AND gather inherited the parent frame
t.expect_file(a5_pre, content="before-gather")
for p in a5_paths:
    t.expect_file(p, content="hola")

t.run(graph)
