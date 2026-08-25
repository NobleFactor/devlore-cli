# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_function_call_walk_tree.star — end-to-end coverage for function.Provider.
#
# `function.call` is the provider's only method, and nothing exercised it. The function resource appeared
# solely as a callback OTHER providers accept — walk_tree's reducer, choose's lambdas — never as the thing
# being dispatched. function is RoleAction only, so it exists in graph scope alone.
#
# A graph's result is not directly assertable, so each callable's return value is observed by feeding it
# into a write and checking the file.

root = t.tmp("fn_walk")
rendered = t.tmp("rendered.txt")
walked_out = t.tmp("walked.txt")

file.mkdir(path=root, mode=0o755)
file.write_text(destination_path=t.tmp("fn_walk/alpha.txt"), content="alpha", mode=0o644)
file.write_text(destination_path=t.tmp("fn_walk/beta.txt"), content="beta", mode=0o644)

nested = t.tmp("fn_walk/nested")
file.mkdir(path=nested, mode=0o755)
file.write_text(destination_path=t.tmp("fn_walk/nested/gamma.txt"), content="gamma", mode=0o644)

# --- function.call, dispatched as a graph unit ----------------------------------------------------

def render(left, right):
    return left + "-" + right

# The callable is minted as a resource, carried in the document, and invoked at execution. Its return
# value reaches the write through a promise.
called = plan.function.call(render, "left", "right")
wrote_render = plan.file.write_text(destination_path=rendered, content=called, mode=0o600)

# NOT covered, deliberately: feeding a promise INTO function.call. `plan.function.call(summarize, walked)`
# hands the callable an unresolved Invocation, because promise resolution operates on declared parameters
# and `*args` is a catch-all with neither name nor type. Filed as #663; extend this fixture when it is
# fixed rather than pinning the defect here.

# --- the same resource kind, dispatched by another provider ---------------------------------------

# walk_tree takes the callable as a reducer rather than calling it directly. The reducer folds the tree
# into one string so the promise can flow into a write.
def collector(initial, resource, path, stack):
    if initial == None:
        return path
    return initial + "," + path

walked = plan.file.walk_tree(root=root, fn=collector, include_gitignored=True)
wrote_walk = plan.file.write_text(destination_path=walked_out, content=walked, mode=0o600)

# Consumers first: ordering comes from the promises, not from list position.
graph = plan.assemble_definition([wrote_render, wrote_walk, called, walked])

t.expect_unit_count(4)
t.expect_file(rendered, content="left-right")

t.run(graph)
