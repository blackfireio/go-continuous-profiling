package profiler

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type Option func(*config)

type HTTPClientInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

type config struct {
	httpClient     HTTPClientInterface
	cpuDuration    time.Duration
	period         time.Duration
	uploadTimeout  time.Duration
	cpuProfileRate int
	agentSocket    string
	agentEndpoint  string
	types          []ProfileType
	labels         map[string]string
	serverId       string
	serverToken    string
}

type logrecorder struct {
	mu   sync.Mutex
	logs []string
}

var (
	logLevels = map[int]zerolog.Level{
		4: zerolog.DebugLevel,
		3: zerolog.InfoLevel,
		2: zerolog.WarnLevel,
		1: zerolog.ErrorLevel,
	}
	DefaultProfileTypes = []ProfileType{CPUProfile}

	log         zerolog.Logger
	logRecorder logrecorder
)

const (
	DefaultCPUDuration   = 45 * time.Second
	defaultPeriod        = 45 * time.Second
	DefaultLogLevel      = zerolog.ErrorLevel
	DefaultUploadTimeout = 10 * time.Second
)

func logLevel(v int) zerolog.Level {
	if v < 1 {
		v = 1
	} else if v > 4 {
		v = 4
	}
	return logLevels[v]
}

func (l *logrecorder) Output() string {
	return strings.Join(l.logs, "\n")
}

func (l *logrecorder) Add(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.logs = append(l.logs, msg)
}

func (l *logrecorder) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.logs = l.logs[:0] // keep cap.
}

func (l *logrecorder) Contains(s []string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(s) == 0 {
		return false
	}

	si := 0
	for _, log := range l.logs {
		if strings.Contains(strings.ToLower(log), strings.ToLower(s[si])) {
			si++
			if si == len(s) {
				return true
			}
		}
	}

	return false
}

func newLoggerFromEnv() (zerolog.Logger, error) {
	var (
		level zerolog.Level = DefaultLogLevel
		out                 = os.Stderr
		rerr  error         = nil
	)
	if v := os.Getenv("BLACKFIRE_LOG_LEVEL"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			rerr = fmt.Errorf("invalid log level value.(%s)", v)
		} else {
			level = logLevel(d)
		}
	}

	if v := os.Getenv("BLACKFIRE_LOG_FILE"); v != "" {
		w, err := os.OpenFile(v, os.O_RDWR|os.O_CREATE, 0664)
		if err != nil {
			rerr = fmt.Errorf("could not open log file at %s: %v", v, err)
		} else {
			out = w
		}
	}

	logRecorder.Reset()
	logger := zerolog.New(out).Level(level).With().Timestamp().Caller().Logger()
	zerolog.TimeFieldFormat = time.RFC3339Nano
	return logger, rerr
}

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
	log = logger

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
		"runtime":         "go",
		"runtime_os":      runtime.GOOS,
		"runtime_arch":    runtime.GOARCH,
		"runtime_version": runtime.Version(),
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

// deprecated: use WithCPUDuration
func CPUDuration(d time.Duration) Option {
	return WithCPUDuration(d)
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

// deprecated: use WithCPUProfileRate
func CPUProfileRate(hz int) Option {
	return WithCPUProfileRate(hz)
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

// deprecated: use WithAppName
func AppName(appName string) Option {
	return WithAppName(appName)
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
		log = log.Level(logLevel(d))
	}
}

// this is only used for testing internally to mock the internal HTTP client.
func withHTTPClient(h HTTPClientInterface) Option {
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
		log = log.Hook(h)
	}
}
