module github.com/appnet-org/proxy

go 1.24.0

replace github.com/appnet-org/arpc => ../..

require (
	github.com/appnet-org/arpc v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.27.0
)

require go.uber.org/multierr v1.11.0 // indirect
