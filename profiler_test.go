package profiler

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	pprof_profile "github.com/google/pprof/profile"
	assert "github.com/stretchr/testify/require"
	"mime/multipart"
)

type MockHTTPClient struct {
	ReqCount int
	DoFunc   func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func isLabel(p *multipart.Part) bool {
	return len(p.FileName()) == 0
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
		} else {
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

func logContains(logs []string, s []string) bool {
	if len(s) == 0 {
		return false
	}

	si := 0
	for _, log := range logs {
		if strings.Contains(log, s[si]) {
			si++
			if si == len(s) {
				return true
			}
		}
	}

	return false
}

func TestStartStop(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		done := make(chan bool, 10)
		m := &MockHTTPClient{}
		m.DoFunc = func(req *http.Request) (*http.Response, error) {
			labels, profiles := parseConProfReq(t, req)
			assert.Equal(t, labels["k1"], "v1")
			assert.Equal(t, labels["k2"], "v2")
			assert.Equal(t, len(labels), 7)
			assert.Equal(t, profiles[0].SampleType[1].Type, "cpu")

			done <- true

			return &http.Response{
				StatusCode: 200,
				Body:       nil,
			}, nil
		}

		Start(WithCPUDuration(100*time.Millisecond),
			withHTTPClient(m),
			WithLabels(map[string]string{"k1": "v1", "k2": "v2"}))
		defer Stop()

		err := Start()
		assert.True(t, strings.Contains(err.Error(), "Profiler is already running"))

		select {
		case <-time.After(time.Duration(1 * time.Second)):
			t.Fatal("test timeouted")
		case <-done:
		}
	})

	t.Run("invalidagentresponse", func(t *testing.T) {
		m := &MockHTTPClient{}
		m.DoFunc = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       nil,
			}, nil
		}

		Start(WithCPUDuration(10*time.Millisecond),
			period(10*time.Millisecond),
			withHTTPClient(m),
			withLogLevel(5),
			withLogRecorder())
		defer Stop()

		time.Sleep(500 * time.Millisecond)

		assert.True(t, logRecorder.Contains([]string{"Failed to upload profile"}))
	})

	t.Run("uploadtimeout", func(t *testing.T) {
		done := make(chan bool, 10)
		m := &MockHTTPClient{}
		m.DoFunc = func(req *http.Request) (*http.Response, error) {
			m.ReqCount++
			if m.ReqCount < maxUploadRetries {
				return nil, context.DeadlineExceeded
			} else {
				done <- true
				return &http.Response{
					StatusCode: 200,
					Body:       nil,
				}, nil
			}
		}

		Start(WithCPUDuration(100*time.Millisecond),
			period(100*time.Millisecond),
			withHTTPClient(m),
			withLogLevel(5),
			withLogRecorder())
		defer Stop()

		select {
		case <-time.After(time.Duration(1 * time.Second)):
			t.Fatal("test timeouted")
		case <-done:
		}

		assert.True(t, logRecorder.Contains([]string{"Upload failed: context deadline exceeded.", "upload profile succeeded"}))
	})

	t.Run("stopduringupload", func(t *testing.T) {
		m := &MockHTTPClient{}
		m.DoFunc = func(req *http.Request) (*http.Response, error) {
			<-activeProfiler.exitCh

			return nil, context.DeadlineExceeded
		}

		Start(WithCPUDuration(100*time.Millisecond),
			period(100*time.Millisecond),
			withHTTPClient(m),
			withLogLevel(5),
			withLogRecorder())
		time.Sleep(150 * time.Millisecond) // wait a bit
		Stop()

		assert.True(t, logRecorder.Contains([]string{"Profile started for", "profiler interrupted!"}))
		assert.False(t, logRecorder.Contains([]string{"Upload profile succeeded"}))
	})

	t.Run("stopduringprofiling", func(t *testing.T) {
		m := &MockHTTPClient{}
		m.DoFunc = func(req *http.Request) (*http.Response, error) {
			<-activeProfiler.exitCh

			return nil, context.DeadlineExceeded
		}

		Start(withHTTPClient(m),
			withLogLevel(5),
			withLogRecorder())
		time.Sleep(200 * time.Millisecond) // wait a bit
		Stop()

		assert.True(t, logRecorder.Contains([]string{"Profile started", "Profile ended", "Profiler interrupted!"}))
	})

	t.Run("uploadqueuefull", func(t *testing.T) {
		m := &MockHTTPClient{}
		m.DoFunc = func(req *http.Request) (*http.Response, error) {
			time.Sleep(1000 * time.Millisecond)
			return &http.Response{
				StatusCode: 200,
				Body:       nil,
			}, nil
		}

		Start(WithCPUDuration(200*time.Millisecond),
			period(200*time.Millisecond),
			withHTTPClient(m),
			withLogLevel(5),
			withLogRecorder())
		time.Sleep(2000 * time.Millisecond) // wait a bit
		Stop()

		assert.True(t, logRecorder.Contains([]string{"Profile started", "Upload queue is full.", "Profiler interrupted!"}))
	})

	t.Run("config", func(t *testing.T) {
		os.Setenv("BLACKFIRE_CONPROF_PERIOD", "11")
		Start()
		assert.Equal(t, activeProfiler.cfg.period, 11*time.Second)
		Stop()
		assert.Nil(t, activeProfiler)
		os.Unsetenv("BLACKFIRE_CONPROF_PERIOD")

		// code overrides env
		os.Setenv("BLACKFIRE_CONPROF_PERIOD", "11")
		Start(period(4 * time.Second))
		assert.Equal(t, activeProfiler.cfg.period, 4*time.Second)
		Stop()
		os.Unsetenv("BLACKFIRE_CONPROF_PERIOD")

		// invalid config
		os.Setenv("BLACKFIRE_CONPROF_PERIOD", "abc")
		err := Start()
		assert.NotNil(t, activeProfiler)
		assert.Nil(t, err)
		assert.Equal(t, activeProfiler.cfg.period, defaultPeriod)
		Stop()
		os.Unsetenv("BLACKFIRE_CONPROF_PERIOD")

		// invalid log config (should default)
		os.Setenv("BLACKFIRE_LOG_LEVEL", "abc")
		err = Start(withLogRecorder())
		assert.Nil(t, err)
		assert.NotNil(t, activeProfiler)
		assert.Equal(t, log.GetLevel(), DefaultLogLevel)
		Stop()
		os.Unsetenv("BLACKFIRE_LOG_LEVEL")
	})
}
