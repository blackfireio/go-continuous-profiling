package profiler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type profiler struct {
	cfg *config

	exitCh   chan struct{}
	uploadCh chan profileBatch
	stopOnce sync.Once
	wg       sync.WaitGroup // wait all goroutines during exit
}

const (
	agentConProfUri = "profiling/v1/input"
)

var (
	mu                sync.Mutex
	activeProfiler    *profiler
	uploadChannelSize = 5
	maxUploadRetries  = 2
	errOldAgent       = errors.New("Continuous profiling feature requires Blackfire Agent >= 2.13.0")
)

func parseNetworkAddressString(agentSocket string) (network string, address string, err error) {
	re := regexp.MustCompile(`^([^:]+)://(.*)`)
	matches := re.FindAllStringSubmatch(agentSocket, -1)
	if matches == nil {
		err = fmt.Errorf("Could not parse agent socket value: [%v]", agentSocket)
		return
	}
	network = matches[0][1]
	address = matches[0][2]
	return
}

func newProfiler(opts ...Option) (*profiler, error) {
	cfg, err := initDefaultConfig()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(cfg)
	}

	agentLessMode := false
	n, a, err := parseNetworkAddressString(cfg.agentSocket)
	if err != nil {
		return nil, fmt.Errorf("Invalid agent socket. (%s)", cfg.agentSocket)
	}
	if n == "unix" {
		cfg.agentEndpoint = fmt.Sprintf("http://unix/%s", agentConProfUri)
	} else if n == "tcp" {
		cfg.agentEndpoint = fmt.Sprintf("http://%s/%s", a, agentConProfUri)
	} else if n == "http" || n == "https" {
		agentLessMode = true
		cfg.agentEndpoint = cfg.agentSocket + "/" + agentConProfUri
	} else {
		return nil, fmt.Errorf("Invalid agent socket. (%s)", cfg.agentSocket)
	}

	if cfg.httpClient == nil {
		if agentLessMode {
			cfg.httpClient = &http.Client{}
		} else {
			t := &http.Transport{
				Dial: func(proto, addr string) (conn net.Conn, err error) {
					return net.Dial(n, a)
				},
			}
			cfg.httpClient = &http.Client{Transport: t}
		}
	}

	p := &profiler{cfg: cfg,
		exitCh:   make(chan struct{}),
		uploadCh: make(chan profileBatch, uploadChannelSize),
	}

	return p, nil
}

func (p *profiler) interruptibleSleep(d time.Duration) {
	select {
	case <-p.exitCh:
	case <-time.After(d):
	}
}

func (p *profiler) enqueueUpload(b profileBatch) {
	for {
		select {
		case p.uploadCh <- b:
			return
		default:
			select {
			case <-p.uploadCh: // evict oldest to make room
				log.Warn().Msgf("Upload queue is full. Evicting oldest profile to make room.")
			default:
				// eviction should never fail
			}
		}
	}
}

func (p *profiler) run() {
	defer close(p.uploadCh)

	ticker := time.NewTicker(p.cfg.period)
	defer ticker.Stop()

	var (
		m  sync.Mutex
		wg sync.WaitGroup
	)

	for {
		// Run different profiles concurrently and enqueue for upload after all
		// finished. `duration <= period` is ensured during config initialization
		// duration = min(duration, period)
		bat := profileBatch{}
		for _, pt := range p.cfg.types {
			wg.Add(1)
			go func(t ProfileType) {
				defer wg.Done()
				data, err := collectFns[t](p)
				if err != nil {
					log.Debug().Msgf("Error getting %s profile. %v", t.String(), err)
					return
				}
				m.Lock()
				defer m.Unlock()
				bat.profiles = append(bat.profiles, &profile{t: t, data: data})
			}(pt)
		}
		wg.Wait()
		p.enqueueUpload(bat)

		select {
		case <-p.exitCh:
			log.Debug().Msg("Profiler interrupted!")
			return
		case <-ticker.C:
			continue
		}
	}
}

type retriableError struct{ err error }

func (e retriableError) Error() string { return e.err.Error() }

func (p *profiler) doRequest(contentType string, body io.Reader) error {
	funcExit := make(chan struct{})
	defer close(funcExit)

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.uploadTimeout)
	go func() {
		select {
		case <-p.exitCh:
		case <-funcExit:
		}
		cancel()
	}()

	log.Debug().Msgf("Uploading profile to %s", p.cfg.agentEndpoint)

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.agentEndpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	if p.cfg.serverId != "" && p.cfg.serverToken != "" {
		req.SetBasicAuth(p.cfg.serverId, p.cfg.serverToken)
	}

	resp, err := p.cfg.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "malformed HTTP version") {
			return errOldAgent
		}
		return &retriableError{err}
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	} else if resp.StatusCode == 404 {
		return errOldAgent
	}

	return errors.New(resp.Status)
}

func (p *profiler) upload() {
	for {
		select {
		case <-p.exitCh:
			return
		case bat := <-p.uploadCh:
			if err := p.doUpload(bat); err != nil {
				log.Error().Msgf("Failed to upload profile: %v", err)
			} else {
				log.Debug().Msgf("Upload profile succeeded.")
			}
		}
	}
}

func (p *profiler) doUpload(b profileBatch) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// write metadata
	for k, v := range p.cfg.labels {
		err := mw.WriteField(k, v)
		if err != nil {
			return err
		}
	}

	for _, pr := range b.profiles {
		formFile, err := mw.CreateFormFile(pr.t.String(), pr.t.String())
		if err != nil {
			return err
		}
		if _, err := formFile.Write(pr.data); err != nil {
			return err
		}
	}

	if err := mw.Close(); err != nil {
		return err
	}

	var err error
	for i := 0; i < maxUploadRetries; i++ {
		select {
		case <-p.exitCh:
			return nil
		default:
		}

		err = p.doRequest(mw.FormDataContentType(), &body)
		if rerr, ok := err.(*retriableError); ok {
			wait := time.Duration(rand.Int63n(p.cfg.period.Nanoseconds()))
			log.Error().Msgf("Profile upload failed: %v. trying again in %s...", rerr, wait)
			p.interruptibleSleep(wait * time.Nanosecond)
			continue
		}
		return err
	}
	return fmt.Errorf("Failed after %d retries, last error was: %v", maxUploadRetries, err)
}

func (p *profiler) stop() {
	p.stopOnce.Do(func() {
		close(p.exitCh)
	})
	p.wg.Wait()
}

func Start(opts ...Option) error {
	mu.Lock()
	defer mu.Unlock()

	if activeProfiler != nil {
		return fmt.Errorf("Profiler is already running")
	}
	p, err := newProfiler(opts...)
	if err != nil {
		return err
	}
	activeProfiler = p

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		activeProfiler.run()
	}()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		activeProfiler.upload()
	}()

	return nil
}

func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if activeProfiler != nil {
		activeProfiler.stop()
		activeProfiler = nil
	}
}
