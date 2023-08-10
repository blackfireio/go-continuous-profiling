package profiler

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	dd_profiler "gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var (
	errOldAgent = errors.New("continuous profiling feature requires Blackfire Agent >= 2.13.0")
)

const (
	agentConProfUri = "profiling/v1/input"
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

type bfTransport struct {
	Transport http.RoundTripper
}

func (t *bfTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.Transport.RoundTrip(req)
	if err != nil {
		if strings.Contains(err.Error(), "malformed HTTP version") {
			log.Error().Err(errOldAgent).Send()
			return response, errOldAgent
		}
	}

	// TODO: same logic as below
	// resp, err := p.cfg.httpClient.Do(req)
	// if err != nil {
	// 	if strings.Contains(err.Error(), "malformed HTTP version") {
	// 		return errOldAgent
	// 	}
	// 	return &retriableError{err}
	// }
	// if resp.Body != nil {
	// 	defer resp.Body.Close()
	// }

	// if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
	// 	return nil
	// } else if resp.StatusCode == 404 {
	// 	return errOldAgent
	// }

	// return errors.New(resp.Status)

	return response, err
}

func Start(opts ...Option) error {
	defer os.Unsetenv("DD_TRACE_AGENT_URL")

	cfg, err := newProfilerConfig(opts...)
	if err != nil {
		return err
	}

	n, a, err := parseNetworkAddressString(cfg.agentSocket)
	if err != nil {
		return fmt.Errorf("invalid agent socket. (%s)", cfg.agentSocket)
	}
	if n == "unix" {
		cfg.agentEndpoint = fmt.Sprintf("http://unix/%s", agentConProfUri)
	} else if n == "tcp" {
		cfg.agentEndpoint = fmt.Sprintf("http://%s/%s", a, agentConProfUri)
	} else if n == "http" || n == "https" {
		cfg.agentEndpoint = cfg.agentSocket + "/" + agentConProfUri
	} else {
		return fmt.Errorf("invalid agent socket. (%s)", cfg.agentSocket)
	}

	apiKey := ""
	if cfg.serverId != "" && cfg.serverToken != "" {
		apiKey = fmt.Sprintf("%s:%s", cfg.serverId, cfg.serverToken)
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
		t := &http.Transport{
			Dial: func(proto, addr string) (conn net.Conn, err error) {
				return net.Dial(n, a)
			},
		}
		httpClient = &http.Client{Transport: &bfTransport{Transport: t}}
	}

	err = dd_profiler.Start(
		dd_profiler.WithHTTPClient(httpClient),
		dd_profiler.CPUProfileRate(cfg.cpuProfileRate),
		dd_profiler.WithPeriod(cfg.period),
		dd_profiler.CPUDuration(cfg.cpuDuration),
		dd_profiler.WithTags(mapLabelsToTags(cfg.labels)...),
		dd_profiler.WithUploadTimeout(cfg.uploadTimeout),
		dd_profiler.WithProfileTypes(mapProfTypesToDDProfTypes(cfg.types)...),
		dd_profiler.WithAPIKey(apiKey),
	)
	if err != nil {
		return err
	}

	return nil
}

func Stop() {
	dd_profiler.Stop()
}
