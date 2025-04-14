@0xbf5147bb3b06fa3d;

using Go = import "/go.capnp";

$Go.package("echo_capnp");
$Go.import("github.com/appnet-org/aprc/examples/echo_capnp/capnp");

struct EchoRequest {
  content @0 :Text;
}

struct EchoResponse {
  content @0 :Text;
}

interface EchoService {
  echo @0 (req :EchoRequest) -> (resp :EchoResponse);
}
