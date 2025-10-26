package random

import (
	"math/rand"
	"net"

	"github.com/appnet-org/arpc/pkg/transport/balancer/types"
)

// RandomBalancer implements random load balancing
type RandomBalancer struct {
	types.BaseBalancer
}

// NewRandomBalancer creates a new random balancer
func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{}
}

func (r *RandomBalancer) Pick(host string, ips []net.IP) net.IP {
	if len(ips) == 0 {
		return nil
	}

	r.BaseBalancer.Mu.RLock()
	defer r.BaseBalancer.Mu.RUnlock()

	index := rand.Intn(len(ips))
	return ips[index]
}

func (r *RandomBalancer) Name() string {
	return "random"
}
