package profiler

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/pprof"
)

type ProfileType int
type CollectFn func(p *profiler) ([]byte, error)

type profile struct {
	t    ProfileType
	data []byte
}

type profileBatch struct {
	profiles []*profile
}

const (
	CPUProfile ProfileType = iota
)

var (
	collectFns = map[ProfileType]CollectFn{
		CPUProfile: collectCPUProfile,
	}
)

func (t ProfileType) String() string {
	switch t {
	case CPUProfile:
		return "cpu"
	default:
		return fmt.Sprintf("invalid profile type (%d)", int(t))
	}
}

func collectCPUProfile(p *profiler) ([]byte, error) {
	var buf bytes.Buffer

	if p.cfg.cpuProfileRate != 0 {
		runtime.SetCPUProfileRate(p.cfg.cpuProfileRate)
	}

	if err := pprof.StartCPUProfile(&buf); err != nil {
		log.Error().Msgf("Error starting profile: %v. Skipping this period.", err)
		return nil, err
	}
	log.Debug().Msgf("CPU profile started for %s", p.cfg.cpuDuration)
	p.interruptibleSleep(p.cfg.cpuDuration)
	pprof.StopCPUProfile()
	log.Debug().Msg("CPU profile ended")

	return buf.Bytes(), nil
}
