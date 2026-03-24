# Sample Starlark file for integration tests.

def greet(name):
    return "hello " + name

def add(a, b):
    return a + b

result = greet("world")
