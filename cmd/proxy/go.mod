module github.com/appnet-org/arpc/cmd/proxy

go 1.24.0

replace github.com/appnet-org/arpc => ../..

replace github.com/appnet-org/arpc-sigcomm/kv-store-symphony => /users/xzhu/compiler/compiler/experiments/arpc-sigcomm/kv-store-symphony

require (
	github.com/appnet-org/arpc v0.0.0-20260121062022-8a0f1bc09760
	github.com/appnet-org/arpc-sigcomm/kv-store-symphony v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.27.1
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.1 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)
