package balancer

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/transport/balancer/random"
	"github.com/appnet-org/arpc/pkg/transport/balancer/types"
	"go.uber.org/zap"
)

// Resolver handles DNS resolution and load balancing
type Resolver struct {
	balancer    types.Balancer
	cache       map[string]dnsCacheEntry
	cacheTTL    time.Duration
	cacheEnable bool
	mu          sync.RWMutex
}

type dnsCacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
}

const defaultCacheTTLSeconds uint = 30

// NewResolver creates a new resolver with the specified balancer and cache settings.
func NewResolver(balancer types.Balancer, cacheEnabled bool, cacheTTLSeconds uint) *Resolver {
	var ttl time.Duration
	if cacheTTLSeconds > 0 {
		ttl = time.Duration(cacheTTLSeconds) * time.Second
	}

	return &Resolver{
		balancer:    balancer,
		cache:       make(map[string]dnsCacheEntry),
		cacheTTL:    ttl,
		cacheEnable: cacheEnabled && ttl > 0,
	}
}

// NewResolverWithDefaults creates a new resolver with cache enabled and the default TTL.
func NewResolverWithDefaults(balancer types.Balancer) *Resolver {
	return NewResolver(balancer, true, defaultCacheTTLSeconds)
}

type dnsLookupResult struct {
	ips          []net.IP
	cacheEnabled bool
	cacheHit     bool
}

// ResolveUDPTarget resolves a UDP address string that may be an IP, FQDN, or empty.
// If it's empty or ":port", it binds to 0.0.0.0:<port>. For FQDNs, it uses the configured balancer
// to select an IP from the resolved addresses.
func (r *Resolver) ResolveUDPTarget(addr string) (*net.UDPAddr, error) {
	if addr == "" {
		return &net.UDPAddr{IP: net.IPv4zero, Port: 0}, nil
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// Handle addr like ":11000"
		if after, ok := strings.CutPrefix(addr, ":"); ok {
			portStr = after
			host = ""
		} else {
			return nil, fmt.Errorf("invalid addr %q: %w", addr, err)
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port in %q: %w", addr, err)
	}

	if host == "" {
		return &net.UDPAddr{IP: net.IPv4zero, Port: port}, nil
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return &net.UDPAddr{IP: ip, Port: port}, nil
	}

	// FQDN case: resolve all IPs and use balancer
	result, err := r.lookupIPs(host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %q: %w", host, err)
	}
	if len(result.ips) == 0 {
		return nil, fmt.Errorf("DNS lookup returned no results for %q", host)
	}

	// Log all resolved IPs
	logging.Debug("DNS lookup completed",
		zap.String("host", host),
		zap.Int("ip_count", len(result.ips)),
		zap.Bool("cache_enabled", result.cacheEnabled),
		zap.Bool("cache_hit", result.cacheHit),
		zap.Strings("ips", ipsToStrings(result.ips)))

	// Use the balancer to pick an IP
	chosen := r.balancer.Pick(host, result.ips)
	if chosen == nil {
		return nil, fmt.Errorf("balancer failed to select an IP for %q", host)
	}

	logging.Debug("Balancer selected IP",
		zap.String("balancer", r.balancer.Name()),
		zap.String("original_addr", addr),
		zap.String("selected_ip", chosen.String()),
		zap.Int("port", port))

	return &net.UDPAddr{IP: chosen, Port: port}, nil
}

// DefaultResolver creates a resolver with a random balancer (for backward compatibility)
func DefaultResolver() *Resolver {
	return NewResolverWithDefaults(random.NewRandomBalancer())
}

func (r *Resolver) lookupIPs(host string) (dnsLookupResult, error) {
	useCache := r.cacheEnable && r.cacheTTL > 0
	if useCache {
		r.mu.RLock()
		entry, ok := r.cache[host]
		r.mu.RUnlock()
		if ok {
			remaining := time.Until(entry.expiresAt)
			if remaining > 0 {
				ips := cloneIPs(entry.ips)
				logging.Debug("DNS cache hit",
					zap.String("host", host),
					zap.Duration("ttl_remaining", remaining),
					zap.Int("ip_count", len(ips)))
				return dnsLookupResult{
					ips:          ips,
					cacheEnabled: true,
					cacheHit:     true,
				}, nil
			}
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return dnsLookupResult{cacheEnabled: useCache}, err
	}

	if useCache {
		now := time.Now()
		r.mu.Lock()
		r.cache[host] = dnsCacheEntry{
			ips:       cloneIPs(ips),
			expiresAt: now.Add(r.cacheTTL),
		}
		r.mu.Unlock()
		logging.Debug("DNS cache populated",
			zap.String("host", host),
			zap.Duration("ttl", r.cacheTTL),
			zap.Int("ip_count", len(ips)))
	}

	return dnsLookupResult{
		ips:          ips,
		cacheEnabled: useCache,
		cacheHit:     false,
	}, nil
}

func ipsToStrings(ips []net.IP) []string {
	if len(ips) == 0 {
		return nil
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		out = append(out, ip.String())
	}
	return out
}

func cloneIPs(src []net.IP) []net.IP {
	if len(src) == 0 {
		return nil
	}
	out := make([]net.IP, len(src))
	for i, ip := range src {
		if ip == nil {
			continue
		}
		out[i] = append(net.IP(nil), ip...)
	}
	return out
}
