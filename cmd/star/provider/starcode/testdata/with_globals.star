# File with many global assignments for testing.

load("@stdlib//os", "os")

VERSION = "1.0.0"
DEBUG = True
MAX_RETRIES = 3

def configure(opts):
    return opts

DEFAULT_CONFIG = {"key": "value"}
ENABLED_FEATURES = ["auth", "logging"]
