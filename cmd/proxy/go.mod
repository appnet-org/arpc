module github.com/appnet-org/proxy

go 1.24.0

replace github.com/appnet-org/arpc => ../..

require (
	github.com/appnet-org/arpc v0.0.0-00010101000000-000000000000
	github.com/appnet-org/arpc/benchmark/kv-store-symphony-element v0.0.0-20260101073445-6c01641db464
	go.uber.org/zap v1.27.0
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.1 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

