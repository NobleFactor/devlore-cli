# Starlark file exercising expression types for complexity walker coverage.
# Each function isolates a single expression construct.

def use_ternary(x):
    """Exercises CondExpr (ternary)."""
    return x if x > 0 else -x

def use_lambda():
    """Exercises LambdaExpr."""
    f = lambda x: x + 1
    return f(2)

def use_dict():
    """Exercises DictExpr via walkDictEntries."""
    return {"a": 1, "b": 2}

def use_slice(items):
    """Exercises SliceExpr via walkSlice."""
    return items[1:3:1]

def use_tuple():
    """Exercises TupleExpr via walkExprs."""
    return (1, 2, 3)

def use_list():
    """Exercises ListExpr via walkExprs."""
    return [1, 2, 3]

def use_index(items):
    """Exercises IndexExpr."""
    return items[0]

def use_dot():
    """Exercises DotExpr."""
    d = struct(field = 1)
    return d.field

def use_paren(a, b):
    """Exercises ParenExpr."""
    return (a + b)

def use_unary(x):
    """Exercises UnaryExpr (not)."""
    return not x

def use_call(x):
    """Exercises CallExpr with arguments."""
    return str(x)

def use_while(n):
    """Exercises WhileStmt."""
    i = 0
    while i < n:
        i = i + 1
    return i

def use_nested_def():
    """Exercises nested DefStmt."""
    def inner():
        return 1
    return inner()

def use_return_expr(x, y):
    """Exercises ReturnStmt with a binary expression result."""
    return x + y

def use_assign_expr(x):
    """Exercises AssignStmt with expression on both sides."""
    result = x + 1
    return result
