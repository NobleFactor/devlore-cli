# test_walk_tree_planned.star — Planned mode: walk temp dir via plan.file.walk_tree.
#
# The reducer collects relative paths into a list. The plan is executed
# by the test runner, which creates the thread and runs the graph.

# Set up temp directory with files (immediate — setup only).
dir = t.tmp("walk_plan")
file.mkdir(resource=dir, mode=0o755)
file.write_text(destination=t.tmp("walk_plan/x.txt"), content="x", mode=0o644)
file.write_text(destination=t.tmp("walk_plan/y.txt"), content="y", mode=0o644)

# Plan the walk_tree action with a callable reducer.
def collector(initial, resource, path, stack):
    if initial == None:
        return [path]
    return initial + [path]

plan.file.walk_tree(root=dir, fn=collector, honor_gitignore=False)

# The walk creates one graph node.
t.expect_node_count(1)
