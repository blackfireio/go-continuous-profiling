//go:build config_darwin || (darwin && !arm64)
// +build config_darwin darwin,!arm64

package profiler

const (
	DefaultAgentSocket = "unix:///usr/local/var/run/blackfire-agent.sock"
)
