@0xbf5147bb3b06fa3d;

using Go = import "/go.capnp";

$Go.package("benchmark_capnp");
$Go.import("github.com/appnet-org/arpc/benchmark/serialization/capnp");

struct BenchmarkMessage {
  id @0 :Int32;
  score @1 :Int32;
  username @2 :Text;
  content @3 :Text;
}
