package k8s

const (
	// the default namespace to use for the test suites. This is used when the user
	// does not specify a namespace.
	DefaultNamespace = "harvester-system-perf"

	// default manager name to used for server-side apply operations. This is used
	// to avoid conflicts with other managers.
	DefaultSSAFieldManager = "harvesterhci.io/hvperf"
)
