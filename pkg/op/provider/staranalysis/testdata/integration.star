# Integration test for staranalysis provider.
# test_files is injected by the Go test as a list of absolute paths.

report = staranalysis.analyze(test_files, cfg={
    "hotspots": True,
    "cyclomatic_threshold": 2,
    "cognitive_threshold": 2,
    "with_index": True,
})

result_has_stats = report.stats != None
result_has_complexity = report.complexity != None
result_has_index = report.index != None
result_hotspot_count = len(report.hotspots)

# Signal completion.
result_done = True
