@0xf19eb35d82052732;
using Go = import "/go.capnp";
$Go.package("kv_capnp");
$Go.import("example.com/bench/kv_capnp");

struct GetRequest {
  key @0 :Text;
}

struct SetRequest {
  key @0 :Text;
  value @1 :Text;
}