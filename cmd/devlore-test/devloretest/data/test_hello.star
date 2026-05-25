# test_hello.star — Single-node graph that runs: echo "Hello World!"
graph = plan.assemble([
    plan.shell.exec(command='echo "Hello World!"'),
])
t.expect_unit_count(1)

t.run(graph)
