package profiler

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

type Option func(*config)

type config struct {
	httpClient     *http.Client
	cpuDuration    time.Duration
	period         time.Duration
	uploadTimeout  time.Duration
	cpuProfileRate int
	agentSocket    string
	types          []ProfileType
	labels         map[string]string
	serverId       string
	serverToken    string
}

var (
	DefaultProfileTypes = []ProfileType{CPUProfile}
)

const (
	DefaultCPUDuration   = 45 * time.Second
	defaultPeriod        = 45 * time.Second
	DefaultUploadTimeout = 10 * time.Second
)

func initDefaultConfig() (*config, error) {
	c := &config{
		cpuDuration:   DefaultCPUDuration,
		period:        defaultPeriod,
		uploadTimeout: DefaultUploadTimeout,
		agentSocket:   DefaultAgentSocket,
		types:         DefaultProfileTypes,
	}

	logger, err := newLoggerFromEnv()
	if err != nil {
		logger.Error().Msgf("%v", err)
	}
	setGlobalLogger(logger)

	if v := os.Getenv("BLACKFIRE_AGENT_SOCKET"); v != "" {
		c.agentSocket = v
	}

	if v := os.Getenv("BLACKFIRE_SERVER_ID"); v != "" {
		c.serverId = v
	}

	if v := os.Getenv("BLACKFIRE_SERVER_TOKEN"); v != "" {
		c.serverToken = v
	}

	if v := os.Getenv("BLACKFIRE_CONPROF_CPU_DURATION"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			log.Error().Msgf("Invalid CPU duration value.(%d)", d)
		} else {
			c.cpuDuration = time.Duration(d) * time.Second
		}
	}

	// undocumented
	if v := os.Getenv("BLACKFIRE_CONPROF_PERIOD"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			log.Error().Msgf("Invalid period value.(%d)", d)
		} else {
			c.period = time.Duration(d) * time.Second
		}
	}

	if v := os.Getenv("BLACKFIRE_CONPROF_CPU_PROFILERATE"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			log.Error().Msgf("Invalid CPU profile rate value.(%d)", d)
		} else {
			c.cpuProfileRate = d
		}
	}
	if v := os.Getenv("BLACKFIRE_CONPROF_UPLOAD_TIMEOUT"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			log.Error().Msgf("Invalid upload timeout value.(%d)", d)
		} else {
			c.uploadTimeout = time.Duration(d) * time.Second
		}
	}

	if c.cpuDuration > c.period {
		c.cpuDuration = c.period
	}

	// Populate default labels.
	c.labels = map[string]string{
		"language":        "go",
		"runtime":         "go",
		"runtime_os":      runtime.GOOS,
		"runtime_arch":    runtime.GOARCH,
		"runtime_version": runtime.Version(),
		"probe_version":   Version.String(),
	}

	if hostname, err := os.Hostname(); err == nil {
		c.labels["host"] = hostname
	}

	// Collect more labels from environment variables. Priority matters for the same label name.
	lookup := []struct {
		labelName string
		envVar    string
	}{
		{"application_name", "BLACKFIRE_CONPROF_APP_NAME"},
		{"application_name", "PLATFORM_APPLICATION_NAME"},

		{"project_id", "PLATFORM_PROJECT"},
	}

	for _, entry := range lookup {
		// Don't override
		if _, exists := c.labels[entry.labelName]; exists {
			continue
		}

		if v := os.Getenv(entry.envVar); v != "" {
			c.labels[entry.labelName] = v
		}
	}

	return c, nil
}

func WithCPUDuration(d time.Duration) Option {
	return func(cfg *config) {
		cfg.cpuDuration = d
	}
}

// undocumented
func period(d time.Duration) Option {
	return func(cfg *config) {
		cfg.period = d
	}
}

func WithCPUProfileRate(hz int) Option {
	return func(cfg *config) {
		cfg.cpuProfileRate = hz
	}
}

func WithProfileTypes(types ...ProfileType) Option {
	return func(cfg *config) {
		cfg.types = []ProfileType{} // reset
		cfg.types = append(cfg.types, types...)
	}
}

// Shortcut to set the "application_name" label
func WithAppName(appName string) Option {
	return func(cfg *config) {
		cfg.labels["application_name"] = appName
	}
}

func WithLabels(labels map[string]string) Option {
	return func(cfg *config) {
		// Merge with pre-populated labels
		for name, value := range labels {
			cfg.labels[name] = value
		}
	}
}

func WithAgentSocket(agentSocket string) Option {
	return func(cfg *config) {
		cfg.agentSocket = agentSocket
	}
}

func WithUploadTimeout(d time.Duration) Option {
	return func(cfg *config) {
		cfg.uploadTimeout = d
	}
}

func WithCredentials(serverId string, serverToken string) Option {
	return func(cfg *config) {
		cfg.serverId = serverId
		cfg.serverToken = serverToken
	}
}

func withLogLevel(d int) Option {
	return func(cfg *config) {
		setGlobalLogger(log.Level(logLevel(d)))
	}
}

// this is only used for testing internally to mock the internal HTTP client.
func withHTTPClient(h *http.Client) Option {
	return func(cfg *config) {
		cfg.httpClient = h
	}
}

// this is only used for testing internally to record log output
func withLogRecorder() Option {
	return func(cfg *config) {
		var h = zerolog.HookFunc(func(e *zerolog.Event, level zerolog.Level, msg string) {
			logRecorder.Add(msg)
		})
		setGlobalLogger(log.Hook(h))
	}
}
