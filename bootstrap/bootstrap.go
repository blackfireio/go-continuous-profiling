package bootstrap

import (
	"os"
)

func init() {
	// Disable sending telemetry data
	os.Setenv("DD_INSTRUMENTATION_TELEMETRY_ENABLED", "false")

	// Disable loggingrate - DD profiler only logs same error at some configurable
	// rate by default
	os.Setenv("DD_LOGGING_RATE", "0")
}
