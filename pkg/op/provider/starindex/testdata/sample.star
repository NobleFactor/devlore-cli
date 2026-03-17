# Sample Starlark file for integration tests.

def greet(name):
    """Greet someone by name."""
    return "hello " + name

MESSAGE = "world"

result = greet(MESSAGE)
