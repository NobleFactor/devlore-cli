# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# generate.star - Generate receivers and actions from provider structs
#
# Reads a provider struct's methods via goast.methods(), then calls
# goast.render() to produce planned receivers, graph actions, and
# immediate receivers.
#
# The Provider struct carries directives:
#
# // +devlore:access= controls which artifacts ALL its methods appear in:
#   access=immediate  — immediate receiver only (default if no directive)
#   access=planned    — planned receiver + graph action wrapper
#   access=both       — all three artifacts
#
# Methods carry directives:
#
# // +devlore:defaults param=value,... marks params as optional with defaults
#
# Planners link by convention, not directive: a package type named
# <MethodName>Planner is the method's planner, emitted into the method's
# announcement metadata.
#
# Generated files live in gen/ subpackage with provider import alias.

# Infrastructure methods excluded from code generation -- not starlark-facing. Pack/Unpack are the op.Packer /
# op.Unpacker content-transport seam (graph document content section), dispatched by the framework only.
SKIP_METHODS = [
    "Attr",
    "AttrNames",
    "Freeze",
    "Hash",
    "Pack",
    "ResolveAttr",
    "String",
    "Truth",
    "Type",
    "Unpack",
]

# Template to output filename mapping.
GEN_TEMPLATE_FILES = {
    "provider": "gen/provider.gen.go",
    "receiver_type_test": "gen/receiver_type.gen_test.go",
    "module_test": "gen/module.gen_test.go",
    "action_test": "gen/action.gen_test.go",
    "resource": "gen/resource.gen.go",
    # action_names lands in the PACKAGE ROOT (not gen/) so callers write file.WriteText, not gen.WriteText.
    "action_names": "action_names.gen.go",
    # dependent_type uses dynamic filenames: gen/<type_snake>.gen.go
}

# Local templates shipped with this extension (loaded from templates/ dir).
LOCAL_TEMPLATES = {
    "provider": "provider.gen.go.template",
    "receiver_type_test": "receiver_type.gen_test.go.template",
    "module_test": "module.gen_test.go.template",
    "action_test": "action.gen_test.go.template",
    "resource": "resource.gen.go.template",
    "action_names": "action_names.gen.go.template",
    "dependent_type": "dependent_type.gen.go.template",
}

# Primitive Go types — return types NOT in this set are considered custom.
PRIMITIVE_RETURNS = [
    "string", "bool", "int", "int64", "[]byte", "[]string",
    "error", "(error)",
    "(string, error)", "(bool, error)", "(int, error)", "(int64, error)",
    "([]byte, error)", "([]string, error)",
]

# Allowlist of Starlark types a typed constructor may declare as a resource source — the exported
# go.starlark.net/starlark data types plus the starlark.Value interface. A constraint member outside this set (a Go
# primitive, a provider type) is not registered as a byType source key. Assumes the conventional unaliased `starlark`
# import qualifier.
STARLARK_SOURCE_TYPES = [
    "starlark.NoneType",
    "starlark.Bool",
    "starlark.Int",
    "starlark.Float",
    "starlark.String",
    "starlark.Bytes",
    "*starlark.List",
    "starlark.Tuple",
    "*starlark.Dict",
    "*starlark.Set",
    "*starlark.Function",
    "*starlark.Builtin",
    "starlark.Value",
]

def load_template(name, ext_dir):
    """Load template content by name from the extension's templates/ directory."""
    if name not in LOCAL_TEMPLATES:
        fail("unknown template: " + name)
    path = file.join(ext_dir, "templates", LOCAL_TEMPLATES[name])
    return file.read_text(path)

def to_snake(name):
    """Convert CamelCase to snake_case."""
    result = []
    for i in range(len(name)):
        ch = name[i]
        if ch.isupper():
            if i > 0:
                prev = name[i - 1]
                if prev.islower():
                    result.append("_")
                elif prev.isupper() and i + 1 < len(name) and name[i + 1].islower():
                    result.append("_")
            result.append(ch.lower())
        else:
            result.append(ch)
    return "".join(result)

def access_title(access):
    """Convert an access string to its Go title-case constant suffix.

    'immediate' → 'Immediate', 'planned' → 'Planned', 'both' → 'Both'
    """
    return access[0].upper() + access[1:]

def lc_first(name):
    """Lowercase the first character of a name."""
    if not name:
        return name
    return name[0].lower() + name[1:]

# =============================================================================
# Directive Parsing
# =============================================================================

def struct_access(path):
    """Extract the +devlore:access level from the Provider struct's doc comment.

    Returns 'immediate' if no directive is found (the default).
    """
    doc = goast.type_doc(path)
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:access=" in line:
            idx = line.index("+devlore:access=")
            value = line[idx + len("+devlore:access="):].strip()
            if value not in ["immediate", "planned", "both"]:
                fail("invalid +devlore:access value %r on Provider struct (valid: immediate, planned, both)" % value)
            return value
    return "immediate"

def struct_root(path):
    """Extract the +devlore:root flag from the Provider struct's doc comment.

    The +devlore:root=true directive sets the RoleRoot placement-zone bit on
    the generated AnnounceProvider call, causing the provider's methods to
    surface flat at their access-defined namespace root rather than nested
    under the provider's own name. See Phase 8 D12 for the semantics.

    Returns False if no directive is found (the default — methods surface
    nested under the provider's name).
    """
    doc = goast.type_doc(path)
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:root=" in line:
            idx = line.index("+devlore:root=")
            value = line[idx + len("+devlore:root="):].strip()
            if value not in ["true", "false"]:
                fail("invalid +devlore:root value %r on Provider struct (valid: true, false)" % value)
            return value == "true"
    return False

def parse_defaults(doc, method_name):
    """Parse +devlore:defaults from a method doc comment.

    Returns a dict of param_name → default_value_string, or empty dict.
    Example: '+devlore:defaults gitignore=true'
    → {"gitignore": "true"}

    Syntactic validation (each violation aborts codegen via fail):
      - every pair must contain '='
      - the parameter name (left side) must be non-empty
      - no parameter name may appear more than once

    An empty value (e.g., 'name=') is permitted — it marks the parameter as
    optional with no concrete default, equivalent to writing 'name?' in the
    wire token. compute_param_names_list collapses this case.

    Semantic validation (cross-checked against the method's parameter list)
    happens in build_method_descriptors after the params dict is built.
    """
    result = {}
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:defaults " in line:
            idx = line.index("+devlore:defaults ")
            pairs = line[idx + len("+devlore:defaults "):].strip()
            for pair in pairs.split(","):
                pair = pair.strip()
                if "=" not in pair:
                    fail("method %s: +devlore:defaults pair %r missing '='" % (method_name, pair))
                k, v = pair.split("=", 1)
                k = k.strip()
                v = v.strip()
                if k == "":
                    fail("method %s: +devlore:defaults pair %r has empty parameter name" % (method_name, pair))
                if k in result:
                    fail("method %s: +devlore:defaults specifies %r more than once" % (method_name, k))
                result[k] = v
    return result

def parse_property(doc, method_name):
    """Parse +devlore:property from a method doc comment.

    +devlore:property is a bare presence flag marking a zero-arg, value-returning getter for eager property
    projection (op.ModifierProperty): starlark attribute access invokes the getter and yields its result rather than
    returning a callable builtin. Returns True when the flag is present, False otherwise. The directive takes no
    value or arguments.
    """
    found = False
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if line == "+devlore:property":
            found = True
        elif line.startswith("+devlore:property"):
            fail("method %s: +devlore:property is a bare flag and takes no value or arguments (got %r)" % (method_name, line))
    return found

# =============================================================================
# Type Graph Helpers
# =============================================================================

def is_custom_return(returns):
    """Check if a method return type is a custom type (not a primitive).

    For '(*Sources, error)', returns 'Sources'.
    For '(string, error)', returns ''.
    """
    if returns in PRIMITIVE_RETURNS:
        return ""
    # Strip parens and error suffix: '(*Sources, error)' → '*Sources'
    r = returns
    if r.startswith("(") and r.endswith(")"):
        r = r[1:-1]
    if r.endswith(", error"):
        r = r[:-len(", error")]
    r = r.strip()
    # Strip pointer: '*Sources' → 'Sources'
    if r.startswith("*"):
        return r[1:]
    return ""

def filter_ctx_param(params):
    """Strip a leading framework-injected parameter from the params list.

    When a provider method's first Go parameter is one of the framework-injected types,
    [op.Method.Invoke] auto-fills it and the remaining parameters align with the caller-supplied parameter
    names. The announce map and starlark-facing surface must not list the injected parameter — it is implicit.

    Recognized framework-injected first parameters (mirror [op.NewMethod]'s detection):
      - *op.ActivationRecord — the per-dispatch record carrying Runtime, NodeID, Context (firstParamIsActivation).
      - context.Context      — the per-session cancellation context (legacy; predates ActivationRecord).
    """
    if len(params) > 0 and params[0].type in ("*op.ActivationRecord", "context.Context"):
        return params[1:]
    return params

def validate_activation_floor(methods, type_name):
    """Enforce the step-27 required floor at generation time — the compile-time exit gate.

    Activation-first is REQUIRED for the methods that cannot correctly run without dispatch identity:
      - compensable actions (a *Receipt or *op.RecoveryStack among the returns) — they claim production and
        commit receipts through the activation's Unit;
      - Compensate* companions — the recovery machinery dispatches them with an activation in hand.
    Everything else is PERMITTED to take one (the read side stays exactly as it is today); this check never
    fires for fallible or pure actions. Runs over the UNFILTERED method list so Compensate* companions —
    excluded from the starlark surface — are validated too. pkg/op mirrors this rule at registration
    (op.NewMethod and the compensating-action index) as the backstop for hand-announced types.
    """
    for m in methods:
        if not m.name[0].isupper():
            continue  # unexported helpers are not announced surface
        first_is_activation = len(m.params) > 0 and m.params[0].type == "*op.ActivationRecord"
        if m.name.startswith("Compensate"):
            if not first_is_activation:
                fail("step-27 required floor: compensating action %s.%s must declare *op.ActivationRecord as its first parameter" % (type_name, m.name))
        elif _returns_compensator(m.returns) and "error" in m.returns:
            if not first_is_activation:
                fail("step-27 required floor: compensable action %s.%s (returns %s) must declare *op.ActivationRecord as its first parameter" % (type_name, m.name, m.returns))

def validate_action_name_consts(path, provider, const_names):
    """Fail if an action-name const would collide with a package-level identifier in the provider package.

    The consts live in the package ROOT (action_names.gen.go), so each shares the package's identifier namespace.
    A package-level func or struct type with the same name as an action method is a Go redeclaration error. Detect
    it here and fail loudly with a clear message rather than emit uncompilable code (the compiler is the backstop
    for the rarer cases goast does not surface — non-struct type decls, package-level vars/consts).
    """
    package_level = {}
    for f in goast.funcs(path, ""):
        package_level[f.name] = "func"
    for s in goast.structs(path):
        package_level[s.name] = "type"
    for name in const_names:
        if name in package_level:
            fail("action-name const %q collides with package-level %s %q in %s -- rename one to avoid a Go redeclaration" %
                 (name, package_level[name], name, provider))

def _returns_compensator(returns):
    """Report whether the return tuple carries a compensator (an exact *Receipt or *RecoveryStack token).

    Exact token matching, not substring: `*ReceiptSpec` is a receipt INPUT specification, not a compensator.
    """
    for token in returns.strip("()").split(","):
        t = token.strip()
        if t == "*Receipt" or t == "*op.RecoveryStack" or t.endswith(".Receipt") or t.endswith(".RecoveryStack"):
            return True
    return False

def filter_methods(methods, include_list):
    """Filter methods down to the user-facing public surface.

    Excludes:
      - unexported methods (lowercase first letter)
      - framework methods listed in SKIP_METHODS
      - Compensate<Name> companions (discovered by reflection at runtime)
      - <Name>Planned companions (discovered by reflection at runtime)

    Compensate and Planned companions are not registered as standalone
    starlark-callable actions. They are attached to their forward method
    by methodFromReflectedMethod in pkg/op/receiver_type.go via naming-
    convention reflection lookup. See docs/architecture/4-resource-management.md
    §6.8 "Companion triplet".
    """
    filtered = []
    all_names = {}
    for m in methods:
        all_names[m.name] = True

    for m in methods:
        if m.name[0].islower():
            continue
        if m.name in SKIP_METHODS:
            continue
        if m.name.startswith("Compensate"):
            continue
        if m.name.endswith("Planned"):
            continue
        if include_list and m.name not in include_list:
            continue
        filtered.append(m)
    return filtered, all_names

def build_method_descriptors(methods, all_names, defaults_map, planner_map):
    """Build method descriptor dicts from filtered method list.

    defaults_map: method_name → {param_name: default_value}
    planner_map: method_name → planner type name (Go identifier); missing key means default planner
    """
    descriptors = []
    for m in methods:
        method_defaults = defaults_map.get(m.name, {})
        method_planner = planner_map.get(m.name, "")
        compensable = ("Compensate" + m.name) in all_names
        pure = "error" not in m.returns

        params = []
        for p in filter_ctx_param(m.params):
            default_val = method_defaults.get(p.name, "")
            is_variadic = p.variadic or (p.name == "args" and p.type.startswith("[]"))
            is_kwargs = p.name == "kwargs" and p.type.startswith("map[string]")
            # Variadic and **kwargs params are inherently optional — the caller may always omit positional
            # overflow or extra keyword args. Mirroring the runtime invariant in pkg/op/parameter.go where
            # parseParameterToken sets Parameter.Optional for these forms unconditionally.
            is_optional = is_variadic or is_kwargs or (p.name in method_defaults)
            params.append({
                "name": p.name,
                "type": p.type,
                "variadic": is_variadic,
                "kwargs": is_kwargs,
                "doc": p.doc,
                "optional": is_optional,
                "default": default_val,
            })

        # Semantic validation of +devlore:defaults against this method's parameter list. Every name in
        # method_defaults must correspond to a real param on this method, and that param must not be variadic or
        # **kwargs (Q7 grammar — defaults bind only to named scalar params). The runtime parser
        # (pkg/op/parameter.go:parseParameterToken) repeats these checks as the contract gate, but failing here
        # surfaces the error at make build time with a precise file/method context.
        params_by_name = {p["name"]: p for p in params}
        for default_name in method_defaults:
            target = params_by_name.get(default_name)
            if target == None:
                fail(
                    "method %s: +devlore:defaults names %r but the method has no such parameter" %
                    (m.name, default_name),
                )
            if target.get("variadic"):
                fail("method %s: +devlore:defaults cannot apply to variadic parameter %r" % (m.name, default_name))
            if target.get("kwargs"):
                fail("method %s: +devlore:defaults cannot apply to **kwargs parameter %r" % (m.name, default_name))

        # +devlore:property marks a zero-arg, value-returning getter for eager property projection
        # (op.ModifierProperty): starlark attribute access invokes the getter and yields its result rather than a
        # callable builtin. Opt-in by design — an untagged zero-arg method stays callable. Validate the directive
        # against the signature the generator can see (arity and return shape); side-effect freedom is the author's
        # assertion.
        is_property = parse_property(m.doc, m.name)
        if is_property:
            if len(params) != 0:
                fail("method %s: +devlore:property is only valid on a zero-arg method" % m.name)
            # returnTypeString renders "" for an action, "error" for an error-only fallible action, and the bare
            # value type otherwise. Only a value-returning method (function or fallible function) can be a property.
            if m.returns == "" or m.returns == "error":
                fail("method %s: +devlore:property requires a value-returning method, but it returns %r" % (m.name, m.returns))

        desc = {
            "name": m.name,
            "returns": m.returns,
            "doc": m.doc,
            "params": params,
            "compensable": compensable,
            "pure": pure,
            "property": is_property,
            "planner": method_planner,
            "file": m.file,
            "line": m.line,
        }
        descriptors.append(desc)
    return descriptors

# =============================================================================
# Struct Converter Helpers
# =============================================================================

def go_type_to_kind(go_type):
    """Map a Go type string to a converter field kind."""
    if go_type == "string":
        return "string"
    if go_type == "int":
        return "int"
    if go_type == "int64":
        return "int64"
    if go_type == "bool":
        return "bool"
    if go_type == "[]string":
        return "string_slice"
    return ""

def cross_pkg_converter(pkg_alias, bare_type):
    """Build a cross-package converter function name: statstatsgen.StatsToStarlark."""
    return pkg_alias + "gen." + bare_type + "ToStarlark"

def cross_pkg_import_info(pkg_alias):
    """Build a cross-package import info dict for a sibling provider."""
    return {"alias": pkg_alias + "gen", "pkg": pkg_alias}

def build_converter_field(field, structs_by_name):
    """Build a single converter field descriptor from a struct field."""
    kind = go_type_to_kind(field.type)
    snake = to_snake(field.name)

    if kind:
        return {
            "go_name": field.name,
            "snake_name": snake,
            "kind": kind,
        }

    # Pointer to struct: *Stats or *starstats.Stats → struct_ptr
    if field.type.startswith("*"):
        inner = field.type[1:]
        if inner in structs_by_name:
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_ptr",
                "converter": inner + "ToStarlark",
                "nullable": True,
                "nil_expr": "starlark.None",
            }
        if "." in inner:
            pkg_alias, bare = inner.split(".", 1)
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_ptr",
                "converter": cross_pkg_converter(pkg_alias, bare),
                "nullable": True,
                "nil_expr": "starlark.None",
                "cross_pkg_import": cross_pkg_import_info(pkg_alias),
            }

    # Slice of struct: []T or []pkg.T → struct_slice
    if field.type.startswith("[]"):
        elem = field.type[2:]
        if elem in structs_by_name:
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_slice",
                "converter": elem + "ToStarlark",
            }
        if "." in elem:
            pkg_alias, bare = elem.split(".", 1)
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_slice",
                "converter": cross_pkg_converter(pkg_alias, bare),
                "cross_pkg_import": cross_pkg_import_info(pkg_alias),
            }

    # Direct struct value: T or pkg.T → struct_value
    if field.type in structs_by_name:
        return {
            "go_name": field.name,
            "snake_name": snake,
            "kind": "struct_value",
            "converter": field.type + "ToStarlark",
        }
    if "." in field.type:
        pkg_alias, bare = field.type.split(".", 1)
        return {
            "go_name": field.name,
            "snake_name": snake,
            "kind": "struct_value",
            "converter": cross_pkg_converter(pkg_alias, bare),
            "cross_pkg_import": cross_pkg_import_info(pkg_alias),
        }

    return None

def collect_pointer_types(all_data_structs, structs_by_name, dependent_descriptors, provider_methods):
    """Collect all struct types that are referenced as pointers.

    A type needs a pointer receiver in its converter if it appears as:
    - *T return type from a method (dependent type or provider method)
    - *T field in another struct (struct_ptr kind)
    """
    pointer_types = {}

    # From provider method returns: (*T, error) means T is pointer-referenced.
    for desc in provider_methods:
        ret = is_custom_return(desc["returns"])
        if ret and ret in all_data_structs:
            pointer_types[ret] = True

    # From dependent type method returns: (*T, error) means T is pointer-referenced.
    for type_name, descs in dependent_descriptors.items():
        for desc in descs:
            ret = is_custom_return(desc["returns"])
            if ret and ret in all_data_structs:
                pointer_types[ret] = True

    # From struct fields: *T fields mark T as pointer-referenced.
    for struct_name in all_data_structs:
        if struct_name not in structs_by_name:
            continue
        info = structs_by_name[struct_name]
        for field in info.fields:
            if field.type.startswith("*"):
                inner = field.type[1:]
                if inner in all_data_structs:
                    pointer_types[inner] = True

    return pointer_types

def build_converter(struct_name, structs_by_name, pointer_types):
    """Build a converter descriptor for a struct type."""
    if struct_name not in structs_by_name:
        return None

    info = structs_by_name[struct_name]
    fields = []
    for field in info.fields:
        fd = build_converter_field(field, structs_by_name)
        if fd:
            fields.append(fd)

    is_pointer = struct_name in pointer_types
    func_name = struct_name + "ToStarlark"
    starlark_name = to_snake(struct_name)

    return {
        "func_name": func_name,
        "go_type": struct_name,
        "is_pointer": is_pointer,
        "starlark_name": starlark_name,
        "fields": fields,
    }

def collect_type_graph(path, provider_methods, structs_by_name, provider_struct_name):
    """Walk the type graph starting from Provider methods.

    `provider_struct_name` is the provider's own Go struct (e.g. "Provider"); it is seeded into `seen` so the
    property-tag struct scan below never pulls the provider into the dependent-type path — the provider is emitted by
    AnnounceProvider, and a +devlore:property method on it (config.Get) must not also be announced as a value type.

    Returns:
      - dependent_types: list of type names that need HasAttrs wrappers (have methods)
      - data_structs: set of type names that need struct converters (no methods, just data)
    """
    # Find custom return types from Provider methods
    custom_returns = []
    for desc in provider_methods:
        type_name = is_custom_return(desc["returns"])
        if type_name:
            custom_returns.append(type_name)

    dependent_types = []
    data_structs = {}
    seen = {provider_struct_name: True}

    def walk_return_type(type_name):
        if type_name in seen:
            return
        seen[type_name] = True

        # Resource types are handled by the resource template path, not dependent_type.
        if type_name == "Resource":
            return

        # Check if this type has methods (→ dependent type with HasAttrs wrapper)
        type_methods = goast.methods(path, receiver_type=type_name)
        has_methods = False
        for m in type_methods:
            if m.name[0].isupper() and m.name not in SKIP_METHODS:
                has_methods = True
                break

        if has_methods:
            dependent_types.append(type_name)
            # Walk this type's methods for further custom returns
            filtered, _ = filter_methods(type_methods, [])
            for m in filtered:
                sub_type = is_custom_return(m.returns)
                if sub_type:
                    walk_return_type(sub_type)
        else:
            # Data struct — needs converter only
            if type_name in structs_by_name:
                data_structs[type_name] = True

    for t in custom_returns:
        walk_return_type(t)

    # Reach structs the return-walk cannot see — e.g. a Decl implementer surfaced only through an interface-typed
    # field (SourceFile.Decls is []Decl), so no method return points at it. The codegen upside is the
    # +devlore:property tag: a struct carrying one needs its Modifiers emitted, so codegen it regardless of
    # reachability. Types already reached by the walk are in `seen` and skipped.
    for struct_name in structs_by_name:
        if struct_name in seen:
            continue
        for m in goast.methods(path, receiver_type=struct_name):
            if parse_property(m.doc, m.name):
                walk_return_type(struct_name)
                break

    return dependent_types, data_structs

def collect_all_data_structs(dependent_descriptors, data_structs, structs_by_name):
    """Collect all data structs referenced by dependent type methods.

    Walks method returns of dependent types plus transitive struct field references.
    """
    # Start with directly referenced data structs
    all_data = dict(data_structs)

    # Add data structs from dependent type method returns
    for descs in dependent_descriptors.values():
        for desc in descs:
            type_name = is_custom_return(desc["returns"])
            if type_name and type_name in structs_by_name and type_name not in dependent_descriptors:
                all_data[type_name] = True

    # Transitively walk struct fields to find nested struct references.
    # Use iterative expansion since Starlark has no while loops.
    queue = list(all_data.keys())
    for _ in range(100):  # safety limit for transitive closure
        if not queue:
            break
        current = queue[0]
        queue = queue[1:]
        if current not in structs_by_name:
            continue
        info = structs_by_name[current]
        for field in info.fields:
            # Slice of struct: []T
            if field.type.startswith("[]"):
                elem = field.type[2:]
                if elem in structs_by_name and elem not in all_data:
                    all_data[elem] = True
                    queue.append(elem)
            # Pointer to struct: *T
            elif field.type.startswith("*"):
                elem = field.type[1:]
                if elem in structs_by_name and elem not in all_data:
                    all_data[elem] = True
                    queue.append(elem)
            # Direct struct embed: T (if it's a known struct)
            elif field.type in structs_by_name and field.type not in all_data:
                inner = field.type
                all_data[inner] = True
                queue.append(inner)

    return all_data

# =============================================================================
# Resource Detection
# =============================================================================

def detect_resources(path):
    """Detect every Resource type in the package, paired with its constructor.

    A Resource type is identified by its public constructor: any function in the package whose
    signature is `func(*op.RuntimeEnvironment, any) (*T, error)` or `(T, error)` declares T as
    a Resource. The constructor IS the public contract — embedding chains can be transitive
    (e.g., mem.Function embeds mem.Resource which embeds op.ResourceBase) and structural-only
    detection misses those; constructor-signature detection catches every type the package
    publicly exposes as a Resource.

    Returns a list of (struct_name, constructor_name, source_types) triples — one entry per detected Resource;
    source_types is the unambiguous Go source types the resource's typed constructor declares (empty when none).
    Returns the empty list if no matching constructors are found. Fails if multiple constructors
    return the same type.
    """
    funcs = goast.funcs(path)

    results = []
    seen_types = {}
    source_types = {}
    for fn in funcs:
        # A typed constructor (one with type parameters) declares the resource's Go source types via its type set.
        if fn.type_params and fn.name and fn.name[0].isupper():
            produced = _return_type_name(fn)
            if produced:
                for st in _source_types(fn):
                    existing = source_types.get(produced, [])
                    if st not in existing:
                        source_types[produced] = existing + [st]
        type_name = _resource_return_type(fn)
        if not type_name:
            continue
        if type_name in seen_types:
            fail(
                "multiple constructors found for Resource type %s: %s and %s" %
                (type_name, seen_types[type_name], fn.name),
            )
        seen_types[type_name] = fn.name
        results.append((type_name, fn.name))
    return [(type_name, constructor_name, source_types.get(type_name, [])) for type_name, constructor_name in results]

def _resource_return_type(fn):
    """Return the Resource type name fn constructs, or "" if fn isn't a Resource constructor.

    A Resource constructor is an *exported* `func(*op.RuntimeEnvironment, any) (*T, error)` or `(T, error)`.
    Unexported helpers (e.g., `buildCandidate`) that share the same signature are excluded —
    only the package's public contract counts. Returns the bare type name T (no leading `*`).
    """
    if not fn.name or not fn.name[0].isupper():
        return ""
    if len(fn.params) != 2:
        return ""
    if fn.params[0].type != "*op.RuntimeEnvironment":
        return ""
    if fn.params[1].type not in ["any", "interface{}"]:
        return ""

    # A constructor may return the package's resource INTERFACE rather than a pointer to the concrete
    # type it constructs. The permissive claim does: an unasserted claim is satisfied by whatever entry
    # already stands for its identity, which may be a different kind, so the constructor cannot promise
    # a pointer to its own type. Every constructor in the tree returns `*T`, so a non-pointer return is
    # exactly that case — derive T from the constructor's name, which every provider already spells
    # `Discover<T>` / `New<T>`.
    if not fn.returns.startswith("(*"):
        for prefix in ["Discover", "New"]:
            if fn.name.startswith(prefix) and len(fn.name) > len(prefix):
                return fn.name[len(prefix):]

    return _return_type_name(fn)

def _return_type_name(fn):
    """Return the bare type T from a `(*T, error)` or `(T, error)` return signature, or "" otherwise."""
    ret = fn.returns
    if not ret.startswith("(") or not ret.endswith(", error)"):
        return ""
    inner = ret[1:-len(", error)")]
    if inner.startswith("*"):
        inner = inner[1:]
    return inner

def _source_types(fn):
    """Return the Starlark source types fn's type-parameter constraint declares.

    fn is a typed constructor (has type parameters) returning (*T, error); its type-set members are the Go source
    types it constructs from. Only members in STARLARK_SOURCE_TYPES (the go.starlark.net/starlark data types plus
    starlark.Value) become byType source keys; a member outside the allowlist — a Go primitive like string — stays
    target-driven.
    """
    result = []
    for tp in fn.type_params:
        for member in tp.constraint:
            if member not in STARLARK_SOURCE_TYPES:
                continue
            if member not in result:
                result.append(member)
    return result

def _source_type_qualifier(source_type):
    """Return the package qualifier of a source type (e.g. *starlark.Function -> starlark), or "" if built-in."""
    t = source_type
    if t.startswith("*"):
        t = t[1:]
    if "." not in t:
        return ""
    return t.split(".")[0]

def _resolve_import(deps, qualifier):
    """Return the import path whose package qualifier (alias, else last path segment) matches qualifier, or ""."""
    for f in deps.files:
        for imp in f.imports:
            q = imp.alias if imp.alias else imp.path.split("/")[-1]
            if q == qualifier:
                return imp.path
    return ""

def _source_imports(path, source_types):
    """Resolve the import paths the source types reference (e.g. *starlark.Function -> go.starlark.net/starlark).

    Built-in source types (no package qualifier) need none. Returns a sorted, de-duplicated list of import paths.
    """
    if not source_types:
        return []
    deps = goast.deps(path)
    imports = {}
    for st in source_types:
        qualifier = _source_type_qualifier(st)
        if not qualifier:
            continue
        resolved = _resolve_import(deps, qualifier)
        if resolved:
            imports[resolved] = True
    return sorted(imports.keys())

def resource_implementation_name(path, struct_name):
    """Return the receiver type whose methods a Resource of struct_name actually dispatches through.

    A SEALED resource is an exported interface over an unexported struct, so the exported name has no
    receiver methods at all — an interface cannot declare one. Dispatch reflects on the struct, so the
    metadata must describe the struct; deriving it from the interface silently yields none, and every stage
    downstream accepts that quietly (nil metadata is legal, the announced method set is simply empty).

    Naming makes the resolution deterministic rather than a guess: the interface takes the provider's
    headline name, the implementation takes its lowercase form (sealed-provider-resources.md, ruling 3).

    Emptiness is the test, not a type query: only an interface has zero receiver methods, and a struct with
    none has nothing to describe either way — so falling back is correct for both.

    Parameters:
      - path:        the package path.
      - struct_name: the announced Resource type name (e.g., "Resource", "Function").

    Returns:
      the receiver type name to inspect.
    """
    if goast.methods(path, receiver_type=struct_name):
        return struct_name

    implementation = struct_name[0].lower() + struct_name[1:]
    if goast.methods(path, receiver_type=implementation):
        return implementation

    return struct_name

def detect_resource_params(path, struct_name):
    """Detect parameterized methods on the named Resource type.

    Finds exported methods that take parameters and return (T) or (T, error). Methods returning only error
    are excluded (not useful as Starlark callables). Methods with unnamed parameters (_) are excluded
    (cannot be called by name from Starlark).

    Inspects the IMPLEMENTATION rather than the announced name — see [resource_implementation_name]. For an
    unsealed resource the two are the same type and nothing changes.

    Parameters:
      - path:        the package path.
      - struct_name: the announced Resource type name (e.g., "Resource", "Function").

    Returns:
      list of {"name": GoName, "params": [snake_name, ...]} dicts, or [] if none found.
    """
    methods = goast.methods(path, receiver_type=resource_implementation_name(path, struct_name))
    result = []
    for m in methods:
        if m.name[0].islower():
            continue
        if m.name in SKIP_METHODS:
            continue
        if not m.params:
            continue
        # Reject error-only returns.
        if m.returns in ["error", "(error)"]:
            continue
        # Accept (T) or (T, error) returns only.
        ret = m.returns
        if ret.startswith("(") and ret.endswith(")"):
            inner = ret[1:-1]
            parts = [p.strip() for p in inner.split(",")]
            if len(parts) > 2:
                continue
            if len(parts) == 2 and parts[1] != "error":
                continue
        # Skip methods with unnamed parameters.
        has_unnamed = False
        param_names = []
        for p in filter_ctx_param(m.params):
            if p.name == "_" or not p.name:
                has_unnamed = True
                break
            param_names.append(to_snake(p.name))
        if has_unnamed:
            continue
        result.append({"name": m.name, "params": param_names})
    return result

# =============================================================================
# Generation: Gen/ Mode
# =============================================================================

def compute_provider_import(path):
    """Compute the Go import path for the provider package.

    Uses goast.deps() to get the module path, then finds go.mod to compute
    the relative package path.
    """
    deps = goast.deps(path)
    module_path = deps.module_path

    if not module_path:
        fail("could not detect Go module path for " + path)

    # Walk up from path to find go.mod directory
    go_mod_dir = ""
    dir = path
    for _ in range(20):  # safety limit
        if file.exists(file.join(dir, "go.mod")):
            go_mod_dir = dir
            break
        parent = file.parent(dir)
        if parent == dir:
            break
        dir = parent

    if not go_mod_dir:
        fail("could not find go.mod for " + path)

    # Compute relative path from go.mod dir to the provider package.
    if go_mod_dir == "." or go_mod_dir == "":
        rel = path
    elif path.startswith(go_mod_dir + "/"):
        rel = path[len(go_mod_dir) + 1:]
    elif path == go_mod_dir:
        rel = ""
    else:
        fail("provider path %s is not under module root %s" % (path, go_mod_dir))

    if rel:
        return module_path + "/" + rel
    return module_path

def emit_provider_receiver(command, path, provider, struct_short, struct_name, access, root,
                      all_method_names, provider_descriptors,
                      output_dir, write_files):
    """Generate receivers in gen/ mode with type graph walking."""

    pkg = provider
    provider_import = compute_provider_import(path)
    ui.note("Provider import: " + provider_import)

    # -------------------------------------------------------------------------
    # Require ProviderBase embedding
    # -------------------------------------------------------------------------
    embeds_provider_base = False
    structs = goast.structs(path)
    for s in structs:
        if s.name == "Provider":
            for f in s.fields:
                if f.embedded and f.type == "op.ProviderBase":
                    embeds_provider_base = True
    if not embeds_provider_base:
        fail("Provider struct must embed op.ProviderBase")

    # -------------------------------------------------------------------------
    # Parse defaults directives from method docs; link planners by convention
    # -------------------------------------------------------------------------
    structs = goast.structs(path)
    structs_by_name = {}
    for s in structs:
        structs_by_name[s.name] = s

    # Build defaults_map and planner_map for Provider methods. Planners link by
    # convention: a package type named <MethodName>Planner is the method's
    # planner — no directive.
    defaults_map = {}
    planner_map = {}
    for desc in provider_descriptors:
        method_defaults = parse_defaults(desc["doc"], desc["name"])
        if method_defaults:
            defaults_map[desc["name"]] = method_defaults
        planner_name = desc["name"] + "Planner"
        if planner_name in structs_by_name:
            planner_map[desc["name"]] = planner_name

    # -------------------------------------------------------------------------
    # Walk type graph to find dependent types and data structs
    # -------------------------------------------------------------------------
    dependent_types, data_structs = collect_type_graph(path, provider_descriptors, structs_by_name, struct_name)
    ui.note("Dependent types: " + str(dependent_types))
    ui.note("Data structs: " + str(list(data_structs.keys())))

    # -------------------------------------------------------------------------
    # Build dependent type method descriptors
    # -------------------------------------------------------------------------
    dependent_descriptors = {}
    for type_name in dependent_types:
        type_methods = goast.methods(path, receiver_type=type_name)
        filtered, dep_all_names = filter_methods(type_methods, [])

        # Parse defaults for dependent type methods
        dep_defaults = {}
        for m in filtered:
            md = parse_defaults(m.doc, m.name)
            if md:
                dep_defaults[m.name] = md

        descs = build_method_descriptors(filtered, dep_all_names, dep_defaults, {})
        dependent_descriptors[type_name] = descs

    # -------------------------------------------------------------------------
    # Collect all data structs (transitively)
    # -------------------------------------------------------------------------
    all_data_structs = collect_all_data_structs(dependent_descriptors, data_structs, structs_by_name)
    ui.note("All data structs for converters: " + str(list(all_data_structs.keys())))

    # Data struct returns are handled by WrapReceiver's auto-bridging via
    # classifyReturn → marshalReflect → marshalStruct. No converter annotation needed.

    # -------------------------------------------------------------------------
    # Re-build Provider method descriptors with defaults and planners applied
    # -------------------------------------------------------------------------
    all_methods_raw = goast.methods(path, receiver_type=struct_name)
    validate_activation_floor(all_methods_raw, struct_name)
    filtered_raw, all_names_raw = filter_methods(all_methods_raw, [])
    provider_method_descs = build_method_descriptors(filtered_raw, all_names_raw, defaults_map, planner_map)

    # Data struct returns are handled by WrapReceiver's auto-bridging via
    # classifyReturn → marshalReflect → marshalStruct. No converter annotation needed.

    # -------------------------------------------------------------------------
    # Generate: Provider immediate receiver (gen/immediate.gen.go)
    # -------------------------------------------------------------------------
    namespace = provider
    if access == "planned":
        # Planned providers also get immediate for gen/ mode
        namespace = "plan." + provider

    # Collect cross-package imports from provider method result_exprs and struct_params
    provider_cross_imports = collect_cross_pkg_imports(provider_import, [], [provider_method_descs])

    provider_desc = {
        "package": pkg,
        "provider": provider,
        "struct_name": struct_short,
        "namespace": namespace,
        "impl_type": struct_name,
        "registered": True,
        "provider_import": provider_import,
        "methods": provider_method_descs,
        "all_methods": list(all_names_raw.keys()),
        "access": access,
        "access_title": access_title(access),
        "root": root,
    }
    if provider_cross_imports:
        provider_desc["cross_package_imports"] = provider_cross_imports

    emit_file(command, "provider", provider_desc, "gen/provider.gen.go",
             struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate receiver type tests (always — type descriptor exists for all providers).
    emit_file(command, "receiver_type_test", provider_desc, "gen/receiver_type.gen_test.go",
             struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate module tests (starlark module protocol).
    if access in ["immediate", "both"]:
        emit_file(command, "module_test", provider_desc, "gen/module.gen_test.go",
                 struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate action tests (action wrappers — dry-run, compensable, undo).
    if access in ["planned", "both"]:
        emit_file(command, "action_test", provider_desc, "gen/action.gen_test.go",
                 struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate action-name consts (step 32) into the PACKAGE ROOT — one op.ActionName per plan-mode action, so
    # callers write plan.Plan(file.WriteText, …) instead of a string literal. Gated on the same access that gives
    # a provider actions; the collision guard fails loudly if a const name shadows a package-level identifier.
    if access in ["planned", "both"]:
        action_const_names = [d["name"] for d in provider_method_descs]
        validate_action_name_consts(path, provider, action_const_names)
        emit_file(command, "action_names", provider_desc, "action_names.gen.go",
                 struct_short, len(provider_method_descs), output_dir, write_files)

    # node_builder_test emission retired with NodeBuilder (Phase 5). Planner-shim
    # tests will be reintroduced when the plan.Provider.Invocation path lands.

    generated_count = 1

    # -------------------------------------------------------------------------
    # Dependent type receivers (gen/<type_snake>.gen.go)
    # -------------------------------------------------------------------------
    for type_name in dependent_types:
        type_snake = to_snake(type_name)
        dep_descs = dependent_descriptors.get(type_name, [])
        dep_desc = {
            "package": pkg,
            "provider": provider,
            "provider_import": provider_import,
            "provider_type_prefix": "provider.",
            "type_name": type_name,
            "starlark_name": type_snake,
            "methods": dep_descs,
        }
        dep_filename = "gen/" + type_snake + ".gen.go"
        emit_file(command, "dependent_type", dep_desc, dep_filename,
                 type_name, len(dep_descs), output_dir, write_files)

    # Struct converters are no longer generated — op.Marshal handles all
    # struct-to-Starlark conversion via reflection.

    # -------------------------------------------------------------------------
    # Generate: Resource descriptors — one gen file per Resource type in the package.
    # -------------------------------------------------------------------------
    for struct_name, constructor_name, source_types in detect_resources(path):
        snake = to_snake(struct_name)
        resource_params = detect_resource_params(path, struct_name)
        resource_desc = {
            "package": pkg,
            "provider": provider,
            "provider_import": provider_import,
            "provider_type_prefix": "provider.",
            "struct_name": struct_name,
            "constructor_name": constructor_name,
            "resource_params": resource_params,
            "source_types": source_types,
            "source_imports": _source_imports(path, source_types),
        }
        emit_file(command, "resource", resource_desc, "gen/" + snake + ".gen.go",
                 struct_name, 1, output_dir, write_files)
        generated_count += 1

    ui.succeed("Done. Generated %d file(s) in gen/ mode for %s" % (generated_count, struct_short))

def collect_cross_pkg_imports(provider_import, converters, method_desc_lists):
    """Collect cross-package imports from converter fields and method result_exprs.

    Returns a list of {"alias": "starstatsgen", "path": "github.com/.../starstats/gen"}.
    """
    if "/" not in provider_import:
        return []

    base = provider_import.rsplit("/", 1)[0]  # e.g., ".../pkg/op/provider"
    imports = {}

    # From converter fields with cross_pkg_import info
    for conv in converters:
        for field in conv.get("fields", []):
            cpkg = field.get("cross_pkg_import")
            if cpkg and cpkg["alias"] not in imports:
                imports[cpkg["alias"]] = base + "/" + cpkg["pkg"] + "/gen"

    # From method descriptors with cross-package result_expr
    for desc_list in method_desc_lists:
        for desc in desc_list:
            expr = desc.get("result_expr", "")
            if "gen." in expr:
                # Extract alias from e.g. "starstatsgen.StatsToStarlark(%s)"
                alias = expr.split(".")[0]
                if alias.endswith("gen") and alias not in imports:
                    pkg = alias[:-3]
                    imports[alias] = base + "/" + pkg + "/gen"

    result = []
    for alias in sorted(imports.keys()):
        result.append({"alias": alias, "path": imports[alias]})
    return result

def annotate_result_exprs(descriptors, data_structs, provider_prefix):
    """Set result_expr on methods whose return type is a data struct or cross-package type.

    Local data structs use converter functions (e.g., IndexToStarlark(result)).
    Cross-package types use qualified converter calls (e.g., starindexgen.IndexToStarlark(result)).

    provider_prefix: if non-empty, prefixed to converter calls for gen/ mode
    (currently not needed since converters live in same package).
    """
    for desc in descriptors:
        type_name = is_custom_return(desc["returns"])
        if type_name and "." in type_name:
            # Cross-package type: starindex.Index → starindexgen.IndexToStarlark
            pkg_alias, bare = type_name.split(".", 1)
            converter = cross_pkg_converter(pkg_alias, bare)
            desc["result_expr"] = converter + "(%s)"
        elif type_name and type_name in data_structs:
            converter = type_name + "ToStarlark"
            desc["result_expr"] = converter + "(%s)"

# =============================================================================
# Pre-computation Helpers for goast.render()
# =============================================================================

def compute_provider_type_prefix(desc):
    """Return 'provider.' for gen/ subpackage mode, '' for same-package."""
    if desc.get("provider_import", ""):
        return "provider."
    return ""

def compute_param_names_list(method):
    """Pre-compute the quoted, comma-separated parameter name list for a method.

    Token grammar emitted to the runtime:
      ("**" | "*")? name ("?" ("=" defaultExpr)?)?

    Branch order: kwargs > variadic > default > optional. A param with both
    "default" and "optional" set takes the default branch — "?=value"
    already encodes optional via the "?". The runtime parses these tokens
    in pkg/op/parameter.go:parseParameterToken.

    A "default" value is emitted inline only when the param's Go type is
    one of the runtime-supported defaultable kinds (bool, int*, uint*,
    float*, string, or a named type whose underlying is one of these).
    Composite types (slice, map, pointer, interface, channel, function)
    fall through to the "?" branch — the runtime's parseDefaultExpression
    cannot parse a literal text against them, and historically these
    directives carry markers like "nil" / "[]" that mean "use Go zero
    value" rather than a real default.
    """
    parts = []
    for p in method.get("params", []):
        name = to_snake(p["name"])
        default = p.get("default", "")
        if p.get("kwargs"):
            name = "**" + name
        elif p.get("variadic"):
            name = "*" + name
        elif default.startswith("{{") and default.endswith("}}"):
            # Deferred-default expression — evaluated at slot-fill via op.DeferredDefault. Bypass the
            # is_simple_defaultable_type filter (the runtime evaluator handles any target type via
            # op.Convert at slot-fill, not parseDefaultExpression's reflect.Kind dispatch). Emit the
            # literal {{ ... }} text verbatim, Go-string-escaped for embedding in the announce-map's
            # Go source string literal.
            escaped = default.replace("\\", "\\\\").replace("\"", "\\\"")
            name += "?=" + escaped
        elif default == "nil":
            # The nil marker means "optional; an absent slot fills with the Go zero value" — for EVERY
            # type, not only the composite kinds is_simple_defaultable_type screens: the runtime cannot
            # parse the literal "nil" against any kind, and an absent optional slot zeroes at fill
            # (op.Convert step 0). Named struct params (e.g. op.OrderingEdge) land here.
            name += "?"
        elif default and is_simple_defaultable_type(p.get("type", "")):
            # Go-string-escape the default expression: backslash first (so subsequent escapes don't double-back),
            # then double-quote. Preserves literal quotes from directives like `severity="warning"` when the
            # token is embedded in a Go source string literal.
            escaped = default.replace("\\", "\\\\").replace("\"", "\\\"")
            name += "?=" + escaped
        elif default or p.get("optional"):
            name += "?"
        parts.append('"' + name + '"')
    return ", ".join(parts)

def is_simple_defaultable_type(go_type):
    """Return True if go_type is structurally suitable for parseDefaultExpression.

    The runtime helper at pkg/op/parameter.go:parseDefaultExpression dispatches
    by reflect.Kind across bool / int* / uint* / float* / string. Composite
    Go types (slice, map, pointer, interface, channel, function, builtin
    "any") are not defaultable from a literal text token; codegen drops them
    to the optional-only "?" form rather than emitting "?=value" the runtime
    cannot parse.
    """
    if go_type.startswith("[]"):
        return False
    if go_type.startswith("map["):
        return False
    if go_type.startswith("*"):
        return False
    if go_type.startswith("chan"):
        return False
    if go_type.startswith("func"):
        return False
    if go_type == "interface{}" or go_type == "any":
        return False
    return True

def compute_provider_init(desc):
    """Pre-compute the ImmediateFactory body code.

    Generates the Go code that constructs an empty provider and delegates to New<StructName><WrapperSuffix>.
    """
    prefix = compute_provider_type_prefix(desc)
    struct_name = desc["struct_name"]
    wrapper_suffix = desc.get("wrapper_suffix", "Receiver")

    return "\t\t\treturn New%s%s(&%sProvider{})" % (struct_name, wrapper_suffix, prefix)

def compute_descriptor_init(desc):
    """Pre-compute the NewImmediate method body for the provider descriptor.

    Same shape as compute_provider_init but with single-tab indentation (method body level, not nested inside a
    closure).
    """
    prefix = compute_provider_type_prefix(desc)
    struct_name = desc["struct_name"]
    wrapper_suffix = desc.get("wrapper_suffix", "Receiver")

    return "\treturn New%s%s(&%sProvider{})" % (struct_name, wrapper_suffix, prefix)

def prepare_render_data(descriptor, template_name):
    """Prepare a descriptor dict for goast.render().

    Pre-computes template function values and adds derived fields.
    Returns render_data.
    """
    # Shallow copy to avoid mutating the original
    desc = dict(descriptor)

    # Apply defaults for optional fields
    if not desc.get("wrapper_suffix", ""):
        desc["wrapper_suffix"] = "Receiver"

    # Pre-compute provider type prefix
    desc["provider_type_prefix"] = compute_provider_type_prefix(desc)

    # Pre-compute descriptor fields for provider template
    if template_name == "provider":
        access = desc.get("access", "immediate")
        root = desc.get("root", False)
        desc["has_actions"] = access in ["planned", "both"]
        desc["has_planned"] = access in ["planned", "both"]
        desc["has_immediate"] = access in ["immediate", "both"]
        if access == "immediate":
            roles = "op.RoleModule"
        elif access == "planned":
            roles = "op.RoleAction"
        else:
            roles = "op.RoleModule|op.RoleAction"
        if root:
            roles = roles + "|op.RoleRoot"
        desc["roles"] = roles

    # Add derived fields to each method. Sort by name first so the emitted op.MethodMetadata map is deterministic:
    # Go map iteration order is randomized, so without a stable sort the generated *.gen.go reshuffle on every run.
    methods = sorted(desc.get("methods", []), key = lambda m: m["name"])
    enriched = []
    for m in methods:
        md = dict(m)
        md["snake_name"] = to_snake(m["name"])
        md["param_names_list"] = compute_param_names_list(m)
        enriched.append(md)
    desc["methods"] = enriched

    return desc

def emit_file(command, template_name, descriptor, filename, label, method_count, output_dir, write_files):
    """Generate a single file from a template and descriptor."""
    ui.note("Generating %s for %s (%d items)..." % (template_name, label, method_count))
    template_content = load_template(template_name, command.extension.dir)

    # Pre-compute template values and render via goast.render()
    render_data = prepare_render_data(descriptor, template_name)
    code = goast.render(template=template_content, data=render_data)

    if write_files and output_dir:
        out_path = output_dir + "/" + filename
        # Ensure gen/ subdirectory exists. Explicit modes pending 13.0(f) step 12 (umask deferred-default);
        # without them the slot defaults to FileMode(0) and the written files become inaccessible.
        out_dir = file.parent(out_path)
        if not file.exists(out_dir):
            file.mkdir(out_dir, mode = 0o755)
        file.write_text(out_path, code, mode = 0o644)
        ui.succeed("Wrote " + out_path)
    else:
        ui.note("--- " + filename + " ---")
        ui.note(code)

# =============================================================================
# Entry Point
# =============================================================================

def run(command, ctx):
    """Generate receivers and actions from a provider struct."""

    # -------------------------------------------------------------------------
    # Validate required arguments
    # -------------------------------------------------------------------------
    path = ctx.args.get("source", "").rstrip("/")
    if not path:
        fail("--source is required")

    gen_mode = ctx.args.get("gen", False)

    # All providers use the same struct name
    struct_name = "Provider"

    # -------------------------------------------------------------------------
    # Discover Provider methods (may be absent for resource-only packages)
    # -------------------------------------------------------------------------
    methods = goast.methods(path, receiver_type=struct_name)
    has_provider = len(methods) > 0

    if has_provider:
        filtered, all_method_names = filter_methods(methods, [])
        if len(filtered) == 0:
            fail("no eligible methods after filtering for " + struct_name)
    else:
        # No Provider struct — check for Resource structs before failing.
        resources = detect_resources(path)
        if not resources:
            fail("no Provider struct and no Resource struct in " + path)

        # Resource-only package: emit one gen file per detected Resource type, named after the
        # type (e.g., Resource → gen/resource.gen.go, Function → gen/function.gen.go). The
        # template is parameterized by struct_name + constructor_name; same template, multiple
        # outputs. The Makefile rule for the package must list every output as a grouped target.
        if not gen_mode:
            fail("--gen is required")
        output_dir = ctx.args.get("output", "")
        write_files = ctx.args.get("write", False)
        provider = path.split("/")[-1]
        provider_import = compute_provider_import(path)
        for struct_name, constructor_name, source_types in resources:
            snake = to_snake(struct_name)
            resource_params = detect_resource_params(path, struct_name)
            resource_desc = {
                "package": provider,
                "provider": provider,
                "provider_import": provider_import,
                "provider_type_prefix": "provider.",
                "struct_name": struct_name,
                "constructor_name": constructor_name,
                "resource_params": resource_params,
                "source_types": source_types,
                "source_imports": _source_imports(path, source_types),
            }
            emit_file(command, "resource", resource_desc, "gen/" + snake + ".gen.go",
                     struct_name, 1, output_dir, write_files)
        ui.succeed("Done. Generated %d resource descriptor(s) for %s" % (len(resources), provider))
        return

    ui.note("Found " + str(len(filtered)) + " methods for " + struct_name)

    # -------------------------------------------------------------------------
    # Derive names and access/root from struct directives
    # -------------------------------------------------------------------------
    provider = path.split("/")[-1]
    struct_short = provider.title()
    access = struct_access(path)
    root = struct_root(path)

    ui.note("Provider access: " + access)
    if root:
        ui.note("Provider root: true")

    # -------------------------------------------------------------------------
    # Build basic method descriptors (without defaults applied)
    # -------------------------------------------------------------------------
    all_descriptors = []

    for m in filtered:
        params = []
        for p in filter_ctx_param(m.params):
            # Infer *args from variadic (...T) or slice ([]T) params.
            # Infer **kwargs from map[string]any params.
            is_variadic = p.variadic or (p.type.startswith("[]") and not p.type.startswith("[]byte"))
            is_kwargs = p.type == "map[string]any"
            params.append({
                "name": p.name,
                "type": p.type,
                "variadic": is_variadic,
                "kwargs": is_kwargs,
                "doc": p.doc,
            })
        compensable = ("Compensate" + m.name) in all_method_names
        pure = "error" not in m.returns

        desc = {
            "name": m.name,
            "returns": m.returns,
            "doc": m.doc,
            "params": params,
            "compensable": compensable,
            "pure": pure,
            "file": m.file,
            "line": m.line,
        }
        all_descriptors.append(desc)

    # -------------------------------------------------------------------------
    # Parse common flags
    # -------------------------------------------------------------------------
    output_dir = ctx.args.get("output", "")
    write_files = ctx.args.get("write", False)

    # -------------------------------------------------------------------------
    # Dispatch to gen/ mode
    # -------------------------------------------------------------------------
    if not gen_mode:
        fail("--gen is required")

    emit_provider_receiver(command, path, provider, struct_short, struct_name, access, root,
                      all_method_names, all_descriptors,
                      output_dir, write_files)
