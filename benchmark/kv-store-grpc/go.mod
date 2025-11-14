module github.com/appnet-org/arpc/benchmark/kv-store-grpc

go 1.24.0

require (
	github.com/appnet-org/arpc v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.27.0
	google.golang.org/grpc v1.72.2
	google.golang.org/protobuf v1.36.10
)

require (
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
)

replace github.com/appnet-org/arpc => ../../
