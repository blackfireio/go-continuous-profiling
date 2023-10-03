package profiler

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestDataDogLoggerBridge(t *testing.T) {
	tests := []struct {
		expectedLevel   zerolog.Level
		expectedMessage string
		log             string
	}{
		{zerolog.NoLevel, "", "foo"},
		{zerolog.NoLevel, "", "Datadog Tracer v1.55.0 TRACE: Oops!"},
		{zerolog.DebugLevel, "Oops!", "Datadog Tracer v1.55.0 DEBUG: Oops!"},
		{zerolog.InfoLevel, "foo", "Datadog Tracer v1.0.0 INFO: foo"},
		{zerolog.WarnLevel, "ouch!", "Datadog Tracer 1.0 WARN: ouch!"},
		{zerolog.ErrorLevel, "Uploading profile failed: 500 Internal Server Error. Trying again in 19.976665696s...", "Datadog Tracer v1.55.0 ERROR: Uploading profile failed: 500 Internal Server Error. Trying again in 19.976665696s..."},
	}

	for _, test := range tests {
		level, message := parseDDLog(test.log)

		require.Equal(t, test.expectedLevel, level)
		require.Equal(t, test.expectedMessage, message)
	}
}
