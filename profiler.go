package profiler

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	dd_profiler "gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var (
	mu           sync.Mutex
	activeConfig *config // used for testing
	errOldAgent  = errors.New("continuous profiling feature requires Blackfire Agent >= 2.13.0")
)

const (
	agentConProfUri = "/profiling/v1/input"
)

func parseNetworkAddressString(agentSocket string) (network string, address string, err error) {
	re := regexp.MustCompile(`^([^:]+)://(.*)`)
	matches := re.FindAllStringSubmatch(agentSocket, -1)
	if matches == nil {
		err = fmt.Errorf("could not parse agent socket value: [%v]", agentSocket)
		return
	}
	network = matches[0][1]
	address = matches[0][2]
	return
}

func newProfilerConfig(opts ...Option) (*config, error) {
	cfg, err := initDefaultConfig()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg, nil
}

func Start(opts ...Option) error {
	mu.Lock()
	defer mu.Unlock()

	cfg, err := newProfilerConfig(opts...)
	if err != nil {
		return err
	}
	activeConfig = cfg

	protocol, address, err := parseNetworkAddressString(cfg.agentSocket)
	if err != nil {
		return fmt.Errorf("invalid agent socket. (%s)", cfg.agentSocket)
	}

	var ddOpts []dd_profiler.Option
	if protocol == "http" || protocol == "https" {
		ddOpts = []dd_profiler.Option{
			dd_profiler.WithURL(strings.TrimSuffix(cfg.agentSocket, "/") + agentConProfUri),
			// Unfortunately, it triggers a warning: "WARN: profiler.WithAgentlessUpload is currently for internal usage only and not officially supported."
			dd_profiler.WithAgentlessUpload(),
			// An API key is required by the agent less mode, but we don't use it
			dd_profiler.WithAPIKey("00000000000000000000000000000000"),
		}
	} else if protocol == "tcp" {
		ddOpts = []dd_profiler.Option{
			dd_profiler.WithAgentAddr(strings.TrimSuffix(address, "/")),
		}
	} else if protocol == "unix" {
		ddOpts = []dd_profiler.Option{
			// Connection to the unix socket is handled by our custom HTTP client
			dd_profiler.WithAgentAddr("localhost"),
		}
	} else {
		return fmt.Errorf("invalid agent socket protocol: %v [%v]", protocol, cfg.agentSocket)
	}

	mapLabelsToTags := func(m map[string]string) []string {
		tags := make([]string, 0, len(m))
		for k, v := range m {
			tags = append(tags, fmt.Sprintf("%s:%s", k, v))
		}
		return tags
	}

	mapProfTypesToDDProfTypes := func(m []ProfileType) []dd_profiler.ProfileType {
		dd_prof_types := make([]dd_profiler.ProfileType, 0, len(m))
		for _, v := range m {
			switch v {
			case CPUProfile:
				dd_prof_types = append(dd_prof_types, dd_profiler.CPUProfile)
			case HeapProfile:
				dd_prof_types = append(dd_prof_types, dd_profiler.HeapProfile)
			case GoroutineProfile:
				dd_prof_types = append(dd_prof_types, dd_profiler.GoroutineProfile)
			default:
			}
		}
		return dd_prof_types
	}

	// generate a custom http client for hooking the transport
	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = NewHTTPClient(protocol, address, cfg.serverId, cfg.serverToken)
	}

	ddOpts = append(ddOpts,
		dd_profiler.WithHTTPClient(httpClient),
		dd_profiler.CPUProfileRate(cfg.cpuProfileRate),
		dd_profiler.WithPeriod(cfg.period),
		dd_profiler.CPUDuration(cfg.cpuDuration),
		dd_profiler.WithTags(mapLabelsToTags(cfg.labels)...),
		dd_profiler.WithUploadTimeout(cfg.uploadTimeout),
		dd_profiler.WithProfileTypes(mapProfTypesToDDProfTypes(cfg.types)...),
	)
	if err = dd_profiler.Start(ddOpts...); err != nil {
		return err
	}

	return nil
}

func Stop() {
	mu.Lock()
	defer mu.Unlock()

	activeConfig = nil
	dd_profiler.Stop()
}
