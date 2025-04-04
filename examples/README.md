## 1. Define your `.proto` file

Create `echo.proto`:

```proto
syntax = "proto3";

package pb;
option go_package = "./pb";

service EchoService {
    rpc Echo(EchoRequest) returns (EchoResponse);
}

message EchoRequest {
    string message = 1;
}

message EchoResponse {
    string message = 1;
}
```

---

## 2. Generate Go code

Run the following command to generate both Protobuf types and aRPC stubs:

```bash
protoc \
  --go_out=paths=source_relative:. \
  --aprc_out=paths=source_relative:. \
  echo/proto/echo.proto
```

This will generate:
- Standard Go types from Protobuf definitions (via `protoc-gen-go`)
- aRPC client/server stubs (via `protoc-gen-aprc`) in `*_aprc.pb.go`

---

## 3. Run the client and server

Start the server:

```bash
go run echo/server.go
```

In a **separate terminal**, run the client:

```bash
go run echo/frontend.go
```

## 4. Test

```bash
curl http://localhost:8080?key=hello
```