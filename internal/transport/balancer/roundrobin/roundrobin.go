package roundrobin

import (
	"net"

	"github.com/appnet-org/arpc/internal/transport/balancer/types"
)

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	types.BaseBalancer
	current int
}

// NewRoundRobinBalancer creates a new round-robin balancer
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		current: 0,
	}
}

func (rr *RoundRobinBalancer) Pick(host string, ips []net.IP) net.IP {
	if len(ips) == 0 {
		return nil
	}

	rr.BaseBalancer.Mu.Lock()
	defer rr.BaseBalancer.Mu.Unlock()

	selected := ips[rr.current]
	rr.current = (rr.current + 1) % len(ips)
	return selected
}

func (rr *RoundRobinBalancer) Name() string {
	return "round_robin"
}
