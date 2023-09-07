package profiler

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	dd_trace "gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
)

const (
	DefaultLogLevel = zerolog.ErrorLevel
)

var (
	logLevels = map[int]zerolog.Level{
		4: zerolog.DebugLevel,
		3: zerolog.InfoLevel,
		2: zerolog.WarnLevel,
		1: zerolog.ErrorLevel,
	}

	log         zerolog.Logger
	logRecorder logrecorder
)

func logLevel(v int) zerolog.Level {
	if v < 1 {
		v = 1
	} else if v > 4 {
		v = 4
	}
	return logLevels[v]
}

func setGlobalLogger(logger zerolog.Logger) {
	log = logger
	dd_trace.UseLogger(NewDataDogLoggerBridge(log))

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

type logrecorder struct {
	mu   sync.Mutex
	logs []string
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

type DataDogLoggerBridge struct {
	logger zerolog.Logger
}

func NewDataDogLoggerBridge(logger zerolog.Logger) *DataDogLoggerBridge {
	return &DataDogLoggerBridge{
		logger: logger,
	}
}

func (l *DataDogLoggerBridge) Log(msg string) {
	l.logger.Info().Msg(msg)
}
