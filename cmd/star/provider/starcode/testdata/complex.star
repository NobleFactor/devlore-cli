# Complex Starlark file for complexity testing.
#
# This file contains functions with various control flow
# to exercise the complexity walker.

load("@stdlib//json", "json")
load("@stdlib//yaml", "yaml")

def simple_func():
    return True

def branching(x, y, z):
    """A function with multiple branches."""
    if x > 0:
        if y > 0:
            return x + y
        elif z > 0:
            return x + z
        else:
            return x
    elif y > 0:
        return y
    else:
        return 0

def looping(items):
    total = 0
    for item in items:
        if item > 0:
            total = total + item
        elif item == 0:
            continue
    return total

def complex_logic(a, b, c, d):
    if a and b:
        if c or d:
            for i in range(10):
                if i > 5 and i < 8:
                    return i
    return 0

def with_comprehension(items):
    filtered = [x for x in items if x > 0]
    return filtered

THRESHOLD = 10
MAX_ITEMS = 100
