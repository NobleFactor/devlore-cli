# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# config-show.star - Show merged configuration
#
# Display the merged configuration from star.yaml hierarchy.

def _print_config(value, indent):
    """Recursively print configuration with indentation."""
    prefix = "  " * indent

    if type(value) == "dict":
        for key in value:
            v = value[key]
            if type(v) == "dict":
                print(prefix + key + ":")
                _print_config(v, indent + 1)
            elif type(v) == "list":
                print(prefix + key + ":")
                for item in v:
                    print(prefix + "  - " + str(item))
            else:
                print(prefix + key + ": " + str(v))
    else:
        print(prefix + str(value))

def run(command, ctx):
    """Show the merged configuration and its sources."""
    result = config.show()

    note("Configuration sources:")
    for source in result.sources:
        if source.exists:
            succeed("  " + source.path)
        else:
            note("  " + source.path + " (not found)")

    print("")
    note("Merged configuration:")
    _print_config(result.config, 0)
