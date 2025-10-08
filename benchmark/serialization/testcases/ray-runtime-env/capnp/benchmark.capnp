@0xbf5147bb3b06fa3e;

using Go = import "/go.capnp";

$Go.package("benchmark_capnp");
$Go.import("github.com/appnet-org/arpc/benchmark/serialization/capnp");

struct RuntimeEnvUris {
  workingDirUri @0 :Text;
  pyModulesUris @1 :List(Text);
}

struct RuntimeEnvConfig {
  setupTimeoutSeconds @0 :Int32;
  eagerInstall @1 :Bool;
  logFiles @2 :List(Text);
}

struct RuntimeEnvInfo {
  serializedRuntimeEnv @0 :Text;
  uris @1 :RuntimeEnvUris;
  runtimeEnvConfig @2 :RuntimeEnvConfig;
}
