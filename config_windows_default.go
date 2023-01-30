//go:build config_windows || windows
// +build config_windows windows

package profiler

const (
	DefaultAgentSocket = "tcp://127.0.0.1:8307"
)
