# End-to-end integration test script for starcode provider.
# Exercises: capture, paths, count, index, stats, analyze.
#
# This script is executed by integration_test.go with starcode
# injected as a global. It sets result_* globals for the Go test
# to inspect.

# --- capture ---
sources = starcode.capture("*.star", gitignore=False, include_bzl=False)

result_count = sources.count
result_paths = sources.paths

# --- stats ---
st = sources.stats(with_bytes=True, with_loc=True)

result_stats_file_count = st.totals.file_count
result_stats_total_bytes = st.totals.total_bytes
result_stats_total_loc = st.totals.total_loc
result_stats_total_sloc = st.totals.total_sloc

# Verify per-file stats are accessible
result_stats_first_file_path = st.files[0].path
result_stats_first_file_loc = st.files[0].loc

# --- index ---
idx = sources.index(with_docstrings=True, with_globals=True)

result_index_file_count = idx.totals.file_count
result_index_functions = idx.totals.functions
result_index_loads = idx.totals.loads
result_index_globals = idx.totals.globals

# Verify per-file index structure
result_index_first_file_path = idx.files[0].path
result_index_first_file_fn_count = len(idx.files[0].functions)

# --- analyze with hotspots ---
report = sources.analyze(hotspots=True, cyclomatic_threshold=3, cognitive_threshold=3, with_index=True)

result_report_has_stats = report.stats != None
result_report_has_complexity = report.complexity != None
result_report_has_index = report.index != None
result_hotspot_count = len(report.hotspots)

# --- analyze without index ---
report_no_idx = sources.analyze(hotspots=True, with_index=False)
result_report_no_idx_index_is_none = (report_no_idx.index == None)

# --- index without docstrings ---
idx_no_doc = sources.index(with_docstrings=False, with_globals=False)
result_no_doc_globals_count = idx_no_doc.totals.globals

# --- stats bytes-only ---
st_bytes = sources.stats(with_bytes=True, with_loc=False)
result_bytes_only_loc = st_bytes.totals.total_loc

# --- verify hotspot fields ---
if len(report.hotspots) > 0:
    h = report.hotspots[0]
    result_hotspot_has_file = h.file != ""
    result_hotspot_has_name = h.name != ""
    result_hotspot_has_line = h.line > 0
    result_hotspot_has_cyclomatic = h.cyclomatic > 0
else:
    result_hotspot_has_file = False
    result_hotspot_has_name = False
    result_hotspot_has_line = False
    result_hotspot_has_cyclomatic = False

# Signal completion
result_done = True
