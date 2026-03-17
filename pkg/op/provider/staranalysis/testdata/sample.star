# Sample Starlark file for integration tests.

def classify(x):
    if x > 100:
        return "large"
    elif x > 50:
        return "medium"
    else:
        return "small"

THRESHOLD = 42

def process(items):
    results = []
    for item in items:
        if item > THRESHOLD:
            results.append(classify(item))
    return results
