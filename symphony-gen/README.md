```
go install ./protoc-gen-arpc
go install ./protoc-gen-symphony

protoc --symphony_out=paths=source_relative:.   --arpc_out=paths=source_relative:. example/echo.proto
```