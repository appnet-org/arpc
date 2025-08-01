// Specify the location of proto files here
const PROTO: &str = "../../proto/echo.proto";
fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-changed={PROTO}");
    prost_build::compile_protos(&[PROTO], &["../../proto"])?;
    Ok(())
}
