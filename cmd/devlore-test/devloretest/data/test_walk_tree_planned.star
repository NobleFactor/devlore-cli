# test_walk_tree_planned.star — Planned mode: walk temp dir via plan.file.walk_tree.
#
# The reducer collects relative paths into a list. The plan is executed
# by the test runner, which creates the thread and runs the graph.

# Set up temp directory with files (test context helpers — setup only).
dir = t.tmp("walk_plan")
t.mkdir(dir)
t.write(t.tmp("walk_plan/x.txt"), "x")
t.write(t.tmp("walk_plan/y.txt"), "y")

# Plan the walk_tree action with a callable reducer.
def collector(initial, resource, path, stack):
    if initial == None:
        return [path]
    return initial + [path]

graph = plan.assemble([
    plan.file.walk_tree(root=dir, fn=collector, include_gitignored=True),
])

# The walk creates one graph node.
t.expect_unit_count(1)

t.run(graph)
