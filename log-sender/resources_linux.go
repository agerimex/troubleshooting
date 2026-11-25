package sender

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"
)

// hostMemory is the machine's memory (MemTotal). In a container this is
// still the machine's; the container's limit is memoryLimit.
func hostMemory() (uint64, bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024, err == nil
		}
	}
	return 0, false
}

// readNumber reads a file holding one unsigned number ("max" and others: false).
func readNumber(path string) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(string(bytes.TrimSpace(data)), 10, 64)
	return v, err == nil
}

// noLimit: cgroup v1 reports "no limit" as a huge number.
const noLimit = 1 << 60

// memoryLimit is the memory limit of the process's cgroup (a container's
// mem_limit), cgroup v2 or v1; false without one.
func memoryLimit() (uint64, bool) {
	if v, ok := readNumber("/sys/fs/cgroup/memory.max"); ok {
		return v, true
	}
	if v, ok := readNumber("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok && v < noLimit {
		return v, true
	}
	return 0, false
}

// containerMemory is the memory the process's cgroup uses now (inside a
// container: the container's, page cache included, as the limit counts it).
// Outside a container the root cgroup has no such file: false.
func containerMemory() (uint64, bool) {
	if v, ok := readNumber("/sys/fs/cgroup/memory.current"); ok {
		return v, true
	}
	return readNumber("/sys/fs/cgroup/memory/memory.usage_in_bytes")
}

// residentMemory is the process's resident set (RSS).
func residentMemory() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}
