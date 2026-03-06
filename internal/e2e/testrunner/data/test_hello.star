# test_hello.star — Single-node graph that runs: echo "Hello World!"
plan.shell.exec(command='echo "Hello World!"')
t.expect_node_count(1)
