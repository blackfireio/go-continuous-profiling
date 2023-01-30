//go:build config_darwin_arm64 || (darwin && arm64)
// +build config_darwin_arm64 darwin,arm64

package profiler

const (
	DefaultAgentSocket = "unix:///opt/homebrew/var/run/blackfire-agent.sock"
)
