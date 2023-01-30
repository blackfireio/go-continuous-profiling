Blackfire Continuous Profiler for Go
====================================

Blackfire Continuous Profiler collects and uploads profiles to the Blackfire Agent. It uses
Go's standart library profilers under the hood.


# API

The probe has two functions:

```
func Start(opts ...Option) error {}
func Stop() {}
```

Following `Option`'s can be used with `Start`. 

```go
profiler.Start(CPUDuration(3 * time.Second), 
       Period(2 * time.Second),
       CPUProfileRate(1000),
       WithProfileTypes(CPUProfile, ),   // can have multiple profile types (goroutine, memory...etc)
       WithLabels({                      // labels are the metadata associated with the profiling payload
            "key1": "value1",
            "key2": "value2",
       }),
       WithAgentSocket("unix:///tmp/blackfire-agent.sock"),
       WithUploadTimeout(5 * time.Second), // upload timeout for the profile payload that is sent to the Agents
)
defer profiler.Stop()
```

Above `Option`'s can also be defined via environment variables. API overrides env. vars and env. vars override defaults.

```
BLACKFIRE_AGENT_SOCKET
BLACKFIRE_CONPROF_CPU_DURATION
BLACKFIRE_CONPROF_PERIOD
BLACKFIRE_CONPROF_CPU_PROFILERATE
BLACKFIRE_LOG_LEVEL
BLACKFIRE_LOG_FILE
BLACKFIRE_CONPROF_UPLOAD_TIMEOUT
```
