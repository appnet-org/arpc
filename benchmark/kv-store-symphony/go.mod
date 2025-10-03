module github.com/appnet-org/arpc/benchmark/kv-store-symphony

go 1.23.9

require (
	github.com/appnet-org/arpc v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.27.0
	google.golang.org/protobuf v1.36.6
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.1 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
)

replace github.com/appnet-org/arpc => ../../
