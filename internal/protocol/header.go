package protocol

import (
	"context"
	"errors"
)

// === HeaderCodec ===

type AppNetHeaderCodec struct{}

func (AppNetHeaderCodec) EncodeHeaders(meta map[string]any) ([]byte, error) {
	buf := make([]byte, 16)
	username, ok := meta["username"].(string)
	if !ok {
		username = "" // fallback
	}
	copy(buf, []byte(username)) // padded/truncated to 16 bytes
	return buf, nil
}

func (AppNetHeaderCodec) DecodeHeaders(data []byte) (map[string]any, error) {
	if len(data) < 16 {
		return nil, errors.New("invalid header length")
	}
	username := string(data[:16])
	return map[string]any{"username": username}, nil
}

// === HeaderInjector ===

type AppNetHeaderInjector struct{}

func (AppNetHeaderInjector) Inject(ctx context.Context, req any) (map[string]any, error) {
	username, _ := ctx.Value("username").(string)
	return map[string]any{"username": username}, nil
}

func (AppNetHeaderInjector) Extract(meta map[string]any, ctx context.Context) context.Context {
	if u, ok := meta["username"]; ok {
		ctx = context.WithValue(ctx, "username", u)
	}
	return ctx
}
