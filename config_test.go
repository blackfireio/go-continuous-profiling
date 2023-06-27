package profiler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Setenv("PLATFORM_APPLICATION_NAME", "foo2")
	t.Setenv("BLACKFIRE_CONPROF_APP_NAME", "foo")
	t.Setenv("PLATFORM_PROJECT", "43")

	config, err := initDefaultConfig()
	require.Nil(t, err)

	require.Contains(t, config.labels, "runtime")
	require.Contains(t, config.labels, "runtime_os")
	require.Contains(t, config.labels, "runtime_arch")
	require.Contains(t, config.labels, "runtime_version")
	require.Contains(t, config.labels, "host")
	require.Contains(t, config.labels, "application_name")
	require.Contains(t, config.labels, "project_id")
	require.NotContains(t, config.labels, "user_id")

	require.Equal(t, "go", config.labels["runtime"])
	require.Equal(t, "foo", config.labels["application_name"])
	require.Equal(t, "43", config.labels["project_id"])

	WithLabels(map[string]string{
		"user_id":          "37",
		"application_name": "bar",
	})(config)

	require.Contains(t, config.labels, "user_id")

	require.Equal(t, "37", config.labels["user_id"])
	require.Equal(t, "bar", config.labels["application_name"])
	require.Equal(t, "go", config.labels["runtime"])

	AppName("duh!")(config)

	require.Equal(t, "duh!", config.labels["application_name"])
	require.Equal(t, "go", config.labels["runtime"])
}
