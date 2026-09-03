package schema

// Bench schema ids (contracts section 11 registry). Registered from a
// separate file so the bench can land without editing the shared table.
func init() {
	Registry["saga.bench.run/1"] = "bench/1/run.json"
	Registry["saga.bench.harness/1"] = "bench/1/harness.json"
	Registry["saga.bench.manifest/1"] = "bench/1/manifest.json"
	Registry["saga.bench.report/1"] = "bench/1/report.json"
}
