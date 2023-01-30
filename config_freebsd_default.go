//go:build config_freebsd || freebsd
// +build config_freebsd freebsd

package profiler

const (
	DefaultAgentSocket = "unix:///var/run/blackfire/agent.sock"
)
