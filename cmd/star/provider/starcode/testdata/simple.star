# A simple Starlark file for testing.

load("@stdlib//json", "json")

def greet(name):
    """Return a greeting string."""
    return "Hello, " + name

def add(a, b):
    return a + b

MESSAGE = "hello"
