package profiler

import "fmt"

type version struct {
	Major int
	Minor int
	Patch int
}

// tag is the current release tag. Must be updated manually.
var probeVersion = version{Major: 1, Minor: 0, Patch: 0}

func (v version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}
