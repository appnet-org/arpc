package balancer

import (
	"fmt"

	"github.com/appnet-org/arpc/pkg/transport/balancer/random"
	"github.com/appnet-org/arpc/pkg/transport/balancer/roundrobin"
	"github.com/appnet-org/arpc/pkg/transport/balancer/types"
)

// BalancerType represents different types of load balancers
type BalancerType string

const (
	BalancerTypeRandom     BalancerType = "random"
	BalancerTypeRoundRobin BalancerType = "round_robin"
)

// NewBalancer creates a new balancer of the specified type
func NewBalancer(balancerType BalancerType, config map[string]any) (types.Balancer, error) {
	switch balancerType {
	case BalancerTypeRandom:
		return random.NewRandomBalancer(), nil

	case BalancerTypeRoundRobin:
		return roundrobin.NewRoundRobinBalancer(), nil

	default:
		return nil, fmt.Errorf("unknown balancer type: %s", balancerType)
	}
}

// NewResolverWithBalancerType creates a new resolver with the specified balancer type
func NewResolverWithBalancerType(balancerType BalancerType, config map[string]any) (*Resolver, error) {
	balancer, err := NewBalancer(balancerType, config)
	if err != nil {
		return nil, err
	}
	return NewResolverWithDefaults(balancer), nil
}
