# test_pkg.star — Dry-run: pkg actions create graph nodes.
#
# Validates: plan.pkg.install, plan.pkg.remove, plan.pkg.upgrade, plan.pkg.update,
#            plan.pkg.installed, plan.pkg.not_installed, plan.pkg.version_gte
#            (registration + node creation)

graph = plan.assemble_definition([
    plan.pkg.install(packages=["curl"], manager="", cask=False),
    plan.pkg.remove(packages=["curl"], manager="", cask=False),
    plan.pkg.upgrade(packages=["curl"], manager="", cask=False),
    plan.pkg.update(manager=""),
    plan.pkg.installed(name="curl"),
    plan.pkg.not_installed(name="curl"),
    plan.pkg.version_gte(name="curl", version="1.0"),
])
t.expect_unit_count(7)
