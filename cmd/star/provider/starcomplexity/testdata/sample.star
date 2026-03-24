# Sample Starlark file for integration tests.

def classify(x):
    if x > 100:
        return "large"
    elif x > 50:
        return "medium"
    elif x > 10:
        return "small"
    else:
        return "tiny"

def process(items):
    results = []
    for item in items:
        if item > 0:
            results.append(classify(item))
    return results
