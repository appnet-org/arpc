## 1. Define your `.capnp` file

Create `echo.capnp`:

```capnp
@0xbf5147bb3b06fa3d;

using Go = import "/go.capnp";

$Go.package("echo_capnp");
$Go.import("github.com/appnet-org/arpc/examples/echo_capnp/capnp");

struct EchoRequest {
  content @0 :Text;
}

struct EchoResponse {
  content @0 :Text;
}

interface EchoService {
  echo @0 (req :EchoRequest) -> (resp :EchoResponse);
}
```

## 2. Generate Go code

See [capnp-gen-arpc](../../cmd/capnp-gen-arpc/README.md) for details.


## 3. Run the client and server

Start the server:

```bash
go run server/server.go
```

In a **separate terminal**, run the client:

```bash
go run frontend/frontend.go
```

## 4. Test

```bash
curl http://localhost:8080?key=hello
```