module github.com/appnet-org/arpc/benchmark/kv-store

go 1.23.9

require (
	github.com/appnet-org/arpc v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.36.6
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.1 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	golang.org/x/sync v0.7.0 // indirect
)

replace github.com/appnet-org/arpc => ../../

replace github.com/appnet-org/arpc/benchmark/kv-store/kv-store => ./kv-store
