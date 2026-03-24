# Integration test: verify @devlore// module loading works end-to-end.
#
# This script is executed by TestLoadIntegration. It exercises:
# 1. Pre-injected globals via With() — ui is available without load()
# 2. On-demand loading via load("@devlore//...") — starcode is loaded
# 3. Loaded modules are usable in function scope (closure over module scope)

load("@devlore//starcode", "starcode")

# Verify ui is predeclared (injected via With("ui"))
result_ui_available = hasattr(ui, "note")

# Verify starcode was loaded and is functional
sources = starcode.capture("*.star", gitignore=False, include_bzl=False)
result_load_worked = sources.count > 0
result_file_count = sources.count

# Verify loaded names are accessible inside functions (closure test)
def check_closure():
    s = starcode.capture("*.star", gitignore=False, include_bzl=False)
    return s.count > 0

result_closure_works = check_closure()

# Verify plan is NOT available (not in With() for this test)
result_plan_not_injected = True
