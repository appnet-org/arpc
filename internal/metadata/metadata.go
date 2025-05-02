package metadata

import (
	"context"
	"strings"
)

// Metadata maps string keys to string values
type Metadata map[string]string

// New creates a Metadata from a map[string]string, normalizing keys to lowercase
func New(m map[string]string) Metadata {
	md := make(Metadata, len(m))
	for k, val := range m {
		key := strings.ToLower(k)
		md[key] = val
	}
	return md
}

// Get returns the string value for the given key (case-insensitive)
func (md Metadata) Get(k string) string {
	k = strings.ToLower(k)
	return md[k]
}

// Set sets a key to a given value, replacing any existing one
func (md Metadata) Set(k, val string) {
	k = strings.ToLower(k)
	md[k] = val
}

// Copy returns a shallow copy of the metadata
func (md Metadata) Copy() Metadata {
	out := make(Metadata, len(md))
	for k, v := range md {
		out[k] = v
	}
	return out
}

// Join merges multiple Metadata maps (later ones override earlier keys)
func Join(mds ...Metadata) Metadata {
	out := Metadata{}
	for _, md := range mds {
		for k, v := range md {
			out[k] = v
		}
	}
	return out
}

// --- Context keys ---

type incomingKey struct{}
type outgoingKey struct{}

// NewIncomingContext returns a new context with the given incoming metadata attached.
func NewIncomingContext(ctx context.Context, md Metadata) context.Context {
	return context.WithValue(ctx, incomingKey{}, md.Copy())
}

// FromIncomingContext retrieves incoming metadata from context.
func FromIncomingContext(ctx context.Context) Metadata {
	md, ok := ctx.Value(incomingKey{}).(Metadata)
	if !ok {
		return Metadata{}
	}
	return md.Copy()
}

// NewOutgoingContext returns a new context with the given outgoing metadata attached.
func NewOutgoingContext(ctx context.Context, md Metadata) context.Context {
	return context.WithValue(ctx, outgoingKey{}, md.Copy())
}

// FromOutgoingContext retrieves outgoing metadata from context.
func FromOutgoingContext(ctx context.Context) Metadata {
	md, ok := ctx.Value(outgoingKey{}).(Metadata)
	if !ok {
		return Metadata{}
	}
	return md.Copy()
}

// AppendToOutgoingContext adds or overwrites keys in outgoing metadata.
func AppendToOutgoingContext(ctx context.Context, kv ...string) context.Context {
	if len(kv)%2 != 0 {
		panic("metadata.AppendToOutgoingContext: key-value pairs must be even")
	}
	md := FromOutgoingContext(ctx)
	for i := 0; i < len(kv); i += 2 {
		key := strings.ToLower(kv[i])
		val := kv[i+1]
		md[key] = val
	}
	return NewOutgoingContext(ctx, md)
}
