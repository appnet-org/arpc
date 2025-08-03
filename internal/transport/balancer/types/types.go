package types

import (
	"net"
	"sync"
)

// Balancer defines the interface for load balancing strategies
type Balancer interface {
	// Pick selects an IP address from the given list of resolved IPs
	Pick(host string, ips []net.IP) net.IP

	// Name returns the name of the balancer
	Name() string
}

// BaseBalancer provides common functionality for balancers
type BaseBalancer struct {
	Mu sync.RWMutex
}
