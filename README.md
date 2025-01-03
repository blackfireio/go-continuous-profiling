# Blackfire Continuous Profiler for Go

Blackfire Continuous Profiler continuously collects and uploads profiling data to the Blackfire servers.

Once the profiler is enabled, it collects the relevant profiling information in configurable intervals and uploads this information periodically to the Blackfire Agent. Blackfire Agent then forwards this information to the backend. It does all these in separate goroutines asynchronously. The heavy lifting of the profiler collection is all done by Go standard library profilers: e.g: See https://pkg.go.dev/runtime/pprof for more details

# Prerequisites

Go >=1.18 and Blackfire Agent version >= 2.13.0 is required.

# API

The profiler has two API functions:

```go
func Start(opts ...Option) error {}
func Stop() {}
```

## `func Start(opts ...Option) error`

 - `Start` starts the continuous profiler probe. It collects profiling information and uploads
it to the Agent periodically.

An example using all available `Option`'s that can be used with `Start`:

```go
import (
	profiler "github.com/blackfireio/go-continuous-profiling"
)

profiler.Start(
	profiler.WithAppName("my-app"),
	profiler.WithCPUDuration(3 * time.Second),
	profiler.WithCPUProfileRate(1000),
	profiler.WithProfileTypes(
		profiler.CPUProfile,
		profiler.HeapProfile,
		profiler.GoroutineProfile,
	),
	profiler.WithLabels(map[string]string{
		"key1": "value1",
		"key2": "value2",
	}),
	profiler.WithAgentSocket("unix:///tmp/blackfire-agent.sock"),
	profiler.WithUploadTimeout(5 * time.Second),
)
defer profiler.Stop()
```


- `WithAppName`: Sets the application name. Can also be set via the environment variable `BLACKFIRE_CONPROF_APP_NAME`.
- `WithCPUDuration`: Specifies the length at which to collect CPU profiles.
  The default is 45 seconds. Can also be set via the environment variable `BLACKFIRE_CONPROF_CPU_DURATION`.
- `WithCPUProfileRate`: Sets the CPU profiling rate to Hz samples per second.
  The default is defined by the Go runtime as 100 Hz. Can also be set via the environment
  variable `BLACKFIRE_CONPROF_CPU_PROFILERATE`.
- `WithProfileTypes`: WithProfileTypes sets the profiler types. Multiple profile types can be set.
  The default is `CPUProfile`, availables are `CPUProfile`,  `HeapProfile`, `GoroutineProfile`
- `WithLabels`: Sets custom labels specific to the profile payload that is sent.
- `WithAgentSocket`: Sets the Blackfire Agent's socket. The default is platform dependent
  and uses the same default as the Blackfire Agent.
- `WithUploadTimeout`: Sets the upload timeout of the message that is sent to the Blackfire Agent.
  The default is 10 seconds. Can also be set via the environment variable `BLACKFIRE_CONPROF_UPLOAD_TIMEOUT`.

Note:
If the same parameter is set by both an environment variable and a `Start` call, the explicit
parameter in the `Start` call takes precedence.

There is also some additional configuration that can be done using environment variables:

`BLACKFIRE_LOG_FILE`: Sets the log file. The default is logging to `stderr`.
`BLACKFIRE_LOG_LEVEL`: Sets the log level. The default is logging only errors.

## `func Stop()`

Stops the continuous profiling probe.

# A simple example application

> **_NOTE:_**
We don't need to specify `BLACKFIRE_SOCKET` in any of the steps below. If running on the same
machine both Agent and probe will point to the same address. This is just for example purposes.

1. Run Blackfire Agent (version 2.13.0 and up)

```
BLACKFIRE_SOCKET="tcp://127.0.0.1:8307" blackfire agent --log-level=5
```

2. Get the continuous profiler from the internal repository.

```
go get github.com/blackfireio/go-continuous-profiling
```

3. Save the following code as `main.go` and run as following:

```
BLACKFIRE_LOG_LEVEL=4 BLACKFIRE_AGENT_SOCKET="tcp://127.0.0.1:8307" go run main.go
```

```go
package main

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"time"

	profiler "github.com/blackfireio/go-continuous-profiling"
)

func doSomethingCpuIntensive() {
	md5Hash := func(s string) string {
		h := md5.New()
		io.WriteString(h, s)
		return hex.EncodeToString(h.Sum(nil))
	}
	for i := 0; i < 1_000_000; i++ {
		md5Hash("IDontHaveAnyPurpose")
	}
}

func main() {
	err := profiler.Start(
		profiler.WithAppName("my-app"),
	)
	if err != nil {
		panic("Error while starting Profiler")
	}
	defer profiler.Stop()

	for i := 0; i < 15; i++ {
		doSomethingCpuIntensive()

		time.Sleep(1 * time.Second)
	}
}
```

4. Profiler will send data to the Agent every 45 seconds by default, and Agent will forward it to the Blackfire
backend. Data then can be visualized at https://blackfire.io.
