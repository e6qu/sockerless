package core

import (
	"fmt"
	"io"
	"net"
)

// PortReservation hands a harness a set of distinct ports the operating
// system reports free. Every listener it opens stays open until Release, so
// no two of the harness's own choices coincide: a port probed and released
// early is exactly what the next probe is handed back, which is how a
// backend's address ended up on the port the simulator's DNS listener took
// a moment later. The ports are free at Release; the processes they are for
// are started right after.
type PortReservation struct {
	held []io.Closer
}

// NewPortReservation starts a reservation. Call Release once every port is
// chosen and before the processes that use them start.
func NewPortReservation() *PortReservation {
	return &PortReservation{}
}

// TCP reserves a port free on TCP.
func (r *PortReservation) TCP() int {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(fmt.Sprintf("reserve TCP port: %v", err))
	}
	r.held = append(r.held, l)
	return tcpPort(l)
}

func tcpPort(l net.Listener) int {
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		panic(fmt.Sprintf("reserve TCP port: listener address %v is not TCP", l.Addr()))
	}
	return addr.Port
}

// TCPAndUDP reserves a port free on both TCP and UDP, for a listener (the
// AWS simulator's Route 53 DNS endpoint) that serves one coordinate over
// both protocols.
func (r *PortReservation) TCPAndUDP() int {
	for range 100 {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			panic(fmt.Sprintf("reserve TCP port: %v", err))
		}
		port := tcpPort(l)
		u, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
		if err != nil {
			_ = l.Close()
			continue
		}
		r.held = append(r.held, l, u)
		return port
	}
	panic("no port free on both TCP and UDP after 100 attempts")
}

// Release frees every reserved port.
func (r *PortReservation) Release() {
	for _, c := range r.held {
		_ = c.Close()
	}
	r.held = nil
}
