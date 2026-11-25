//go:build !linux

package sender

// Memory figures are read from /proc and cgroup files, which only Linux has.
// Elsewhere only the CPU time is recorded.

func hostMemory() (uint64, bool)      { return 0, false }
func memoryLimit() (uint64, bool)     { return 0, false }
func containerMemory() (uint64, bool) { return 0, false }
func residentMemory() (uint64, bool)  { return 0, false }
