module github.com/appnet-org/arpc/benchmark/serialization

go 1.23.9

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.1
	github.com/google/flatbuffers v25.2.10+incompatible
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	golang.org/x/sync v0.7.0 // indirect
)

replace github.com/appnet-org/arpc/benchmark/serialization => .
