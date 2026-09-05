package core

import (
	"fmt"
	"net"
	"testing"
)

// TestPortReservationHandsOutDistinctPorts: every port of one reservation is
// distinct, held until Release, and free afterwards.
func TestPortReservationHandsOutDistinctPorts(t *testing.T) {
	r := NewPortReservation()
	ports := []int{r.TCP(), r.TCP(), r.TCPAndUDP(), r.TCP()}
	seen := map[int]bool{}
	for _, p := range ports {
		if seen[p] {
			t.Fatalf("port %d handed out twice in %v", p, ports)
		}
		seen[p] = true
		if l, err := net.Listen("tcp", fmt.Sprintf(":%d", p)); err == nil {
			_ = l.Close()
			t.Fatalf("port %d is not held while reserved", p)
		}
	}
	r.Release()
	for _, p := range ports {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			t.Fatalf("port %d still held after Release: %v", p, err)
		}
		_ = l.Close()
	}
}
