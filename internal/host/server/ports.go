package server

import (
	"fmt"
	"net"
)

// MaxPortProbes caps how many ports a single scan will try to bind.
const MaxPortProbes = 300

// PortFree reports whether 127.0.0.1:port can be bound right now.
func PortFree(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// FreePorts returns up to limit bindable ports in [from, to], trying at most
// MaxPortProbes ports. skip is excluded (e.g. the current port).
func FreePorts(from, to, limit, skip int) []int {
	if from < 1024 {
		from = 1024
	}
	if to > 65535 {
		to = 65535
	}
	if limit <= 0 {
		limit = 20
	}
	free := []int{}
	probes := 0
	for p := from; p <= to && len(free) < limit && probes < MaxPortProbes; p++ {
		if p == skip {
			continue
		}
		probes++
		if PortFree(p) {
			free = append(free, p)
		}
	}
	return free
}
