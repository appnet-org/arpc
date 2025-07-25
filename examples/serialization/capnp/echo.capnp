@0xbf5147bb3b06fa3d;

using Go = import "/go.capnp";

$Go.package("echo_capnp");
$Go.import("github.com/appnet-org/arpc/examples/echo_capnp/capnp");

interface EchoService {
  echo @0 (req :EchoRequest) -> (resp :EchoResponse);
}

struct EchoRequest {
  id @0 :Int32;
  score @1 :Int32;
  username @2 :Text;
  content @3 :Text;
}

struct EchoResponse {
  id @0 :Int32;
  score @1 :Int32;
  username @2 :Text;
  content @3 :Text;
}