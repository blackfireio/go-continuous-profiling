//go:build config_linux || (linux && !config_darwin && !config_darwin_arm64 && !config_windows && !config_freebsd)
// +build config_linux linux,!config_darwin,!config_darwin_arm64,!config_windows,!config_freebsd

package profiler

const (
	DefaultAgentSocket = "unix:///var/run/blackfire/agent.sock"
)
