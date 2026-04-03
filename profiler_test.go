package profiler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mime/multipart"

	pprof_profile "github.com/google/pprof/profile"
	"github.com/klauspost/compress/zstd"
	assert "github.com/stretchr/testify/require"
)

type mockTransport struct {
	//Transport http.RoundTripper

	DoRoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.DoRoundTripFunc(req)
}

func isLabel(p *multipart.Part) bool {
	return len(p.FileName()) == 0
}

func isDataDogEventPart(p *multipart.Part) bool {
	return p.FormName() == "event" && p.FileName() == "event.json"
}

func isPProfFile(p *multipart.Part) bool {
	return filepath.Ext(p.FileName()) == ".pprof" || p.FileName() == "cpu"
}

func parseConProfReq(t *testing.T, r *http.Request) (map[string]string, []*pprof_profile.Profile) {
	reader, err := r.MultipartReader()
	if err != nil {
		t.Fatal(err)
	}

	labels := map[string]string{}
	profiles := []*pprof_profile.Profile{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if part == nil {
			t.Fatal("Invalid part")
		}

		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatal("Invalid part body")
		}

		if isLabel(part) {
			labels[part.FormName()] = string(body)
		} else if isDataDogEventPart(part) {
			// DataDog send a JSON file named "event.json", which contains various metadata, including profile tags (language, application name, ...)
			ddEvent := struct {
				TagsProfiler string `json:"tags_profiler"`
			}{}
			err = json.Unmarshal(body, &ddEvent)
			if err != nil {
				t.Fatal("Cannot decode the 'event.json' file")
				break
			}
			for _, tag := range strings.Split(ddEvent.TagsProfiler, ",") {
				ta := strings.SplitN(tag, ":", 2)
				if len(ta) != 2 {
					t.Fatal("Invalid tag, the string does not contain a key and a value")
					break
				}
				labels[ta[0]] = ta[1]
			}
		} else if isPProfFile(part) {
			// v2 sends zstd-compressed pprof data
			if len(body) >= 4 && body[0] == 0x28 && body[1] == 0xb5 && body[2] == 0x2f && body[3] == 0xfd {
				dec, err := zstd.NewReader(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				body, err = io.ReadAll(dec)
				dec.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			pp, err := pprof_profile.ParseData(body)
			if err != nil {
				t.Fatal(err)
			}

			if err := pp.CheckValid(); err != nil {
				t.Fatal(err)
			}

			profiles = append(profiles, pp)
		}
	}
	return labels, profiles
}

func TestStartStop(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		done := make(chan bool, 10)
		m := &mockTransport{}
		h := &http.Client{Transport: m}
		m.DoRoundTripFunc = func(req *http.Request) (*http.Response, error) {
			labels, profiles := parseConProfReq(t, req)
			assert.Equal(t, labels["k1"], "v1")
			assert.Equal(t, labels["k2"], "v2")
			assert.True(t, len(labels) >= 8)
			assert.Equal(t, profiles[0].SampleType[1].Type, "cpu")

			done <- true

			return &http.Response{
				StatusCode: 200,
				Body:       nil,
			}, nil
		}

		Start(period(100*time.Millisecond),
			WithCPUDuration(100*time.Millisecond),
			withHTTPClient(h),
			WithLabels(map[string]string{"k1": "v1", "k2": "v2"}))
		defer Stop()

		select {
		case <-time.After(time.Duration(1 * time.Second)):
			t.Fatal("test timeouted")
		case <-done:
		}
	})

	for _, protocol := range []string{"http", "https"} {
		t.Run("agent_addr_"+protocol, func(t *testing.T) {
			done := make(chan bool, 10)
			m := &mockTransport{}
			h := &http.Client{Transport: m}
			m.DoRoundTripFunc = func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "/profiling/v1/input", req.URL.Path)
				assert.Equal(t, "localhost:8307", req.URL.Host)

				_, profiles := parseConProfReq(t, req)
				assert.Equal(t, profiles[0].SampleType[1].Type, "cpu")

				done <- true
				return &http.Response{StatusCode: 200, Body: nil}, nil
			}

			Start(period(100*time.Millisecond),
				WithCPUDuration(100*time.Millisecond),
				withHTTPClient(h),
				WithAgentSocket(protocol+"://localhost:8307"))
			defer Stop()

			select {
			case <-time.After(time.Duration(1 * time.Second)):
				t.Fatal("test timeouted")
			case <-done:
			}
		})
	}

	t.Run("config", func(t *testing.T) {
		os.Setenv("BLACKFIRE_CONPROF_PERIOD", "11")
		Start()
		assert.NotNil(t, activeConfig)
		assert.Equal(t, activeConfig.period, 11*time.Second)
		Stop()
		assert.Nil(t, activeConfig)
		os.Unsetenv("BLACKFIRE_CONPROF_PERIOD")

		// code overrides env
		os.Setenv("BLACKFIRE_CONPROF_PERIOD", "11")
		Start(period(4 * time.Second))
		assert.Equal(t, activeConfig.period, 4*time.Second)
		Stop()
		os.Unsetenv("BLACKFIRE_CONPROF_PERIOD")

		// invalid config
		os.Setenv("BLACKFIRE_CONPROF_PERIOD", "abc")
		err := Start()
		assert.Nil(t, err)
		assert.Equal(t, activeConfig.period, defaultPeriod)
		Stop()
		os.Unsetenv("BLACKFIRE_CONPROF_PERIOD")

		// invalid log config (should default)
		os.Setenv("BLACKFIRE_LOG_LEVEL", "abc")
		err = Start(withLogRecorder())
		assert.Nil(t, err)
		assert.NotNil(t, activeConfig)
		assert.Equal(t, log.GetLevel(), DefaultLogLevel)
		Stop()
		os.Unsetenv("BLACKFIRE_LOG_LEVEL")
	})
}
