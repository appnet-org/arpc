module github.com/appnet-org/arpc/examples/echo_capnp

go 1.23.0

toolchain go1.23.9

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.1
	github.com/appnet-org/arpc v0.0.0-20250521234147-524183cf9b99
)

require (
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/appnet-org/arpc => ../..
