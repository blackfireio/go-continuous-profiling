package profiler

import (
	"fmt"
)

type ProfileType int

const (
	CPUProfile ProfileType = iota
	HeapProfile
	GoroutineProfile
)

func (t ProfileType) String() string {
	switch t {
	case CPUProfile:
		return "cpu"
	case HeapProfile:
		return "heap"
	case GoroutineProfile:
		return "goroutine"
	default:
		return fmt.Sprintf("invalid profile type (%d)", int(t))
	}
}
